/**
 * @module sync/merge
 *
 * Three-way merge engine using the diff-match-patch algorithm.
 * Implements a functional subset of Google's diff-match-patch library
 * sufficient for merging concurrent text edits in Obsidian vaults.
 */

/* ------------------------------------------------------------------ */
/*  Types                                                              */
/* ------------------------------------------------------------------ */

/** Operation type: -1 = DELETE, 0 = EQUAL, 1 = INSERT */
type DiffOp = -1 | 0 | 1;

/** A single diff operation: [operation, text] */
type Diff = [DiffOp, string];

const DIFF_DELETE: DiffOp = -1;
const DIFF_EQUAL: DiffOp = 0;
const DIFF_INSERT: DiffOp = 1;

/**
 * Represents a single patch operation in unified diff format.
 */
class Patch {
  /** Diff operations comprising this patch. */
  diffs: Diff[] = [];
  /** Start position in text1. */
  start1: number = 0;
  /** Start position in text2. */
  start2: number = 0;
  /** Length of the affected region in text1. */
  length1: number = 0;
  /** Length of the affected region in text2. */
  length2: number = 0;

  /** Render this patch as a unified diff string. */
  toString(): string {
    let coords1: string;
    if (this.length1 === 0) {
      coords1 = this.start1 + ",0";
    } else if (this.length1 === 1) {
      coords1 = String(this.start1 + 1);
    } else {
      coords1 = this.start1 + 1 + "," + this.length1;
    }

    let coords2: string;
    if (this.length2 === 0) {
      coords2 = this.start2 + ",0";
    } else if (this.length2 === 1) {
      coords2 = String(this.start2 + 1);
    } else {
      coords2 = this.start2 + 1 + "," + this.length2;
    }

    const lines = ["@@ -" + coords1 + " +" + coords2 + " @@\n"];
    for (const [op, text] of this.diffs) {
      let prefix: string = " ";
      switch (op) {
        case DIFF_INSERT:
          prefix = "+";
          break;
        case DIFF_DELETE:
          prefix = "-";
          break;
        case DIFF_EQUAL:
          prefix = " ";
          break;
      }
      lines.push(prefix + encodeURI(text) + "\n");
    }
    return lines.join("");
  }
}

/* ------------------------------------------------------------------ */
/*  DiffMatchPatch                                                     */
/* ------------------------------------------------------------------ */

/**
 * A minimal implementation of Google's diff-match-patch algorithm.
 * Provides diff computation, patch creation, and patch application
 * for three-way text merging.
 */
class DiffMatchPatch {
  /** Number of seconds to map a diff before giving up (0 = infinity). */
  diffTimeout = 1;
  /** Cost of an empty edit operation in terms of edit distance. */
  diffEditCost = 4;
  /** Threshold for match_bitap fuzzy matching (0 = exact, 1 = loose). */
  matchThreshold = 0.5;
  /** Distance from expected location to look for a match. */
  matchDistance = 1000;
  /** Chunk size for context expansion around patches. */
  patchMargin = 4;
  /** Maximum bits supported for bitap matching. */
  matchMaxBits = 32;

  /* ---------------------------------------------------------------- */
  /*  Diff: Public API                                                 */
  /* ---------------------------------------------------------------- */

  /**
   * Find the differences between two texts.
   * @param text1 - Old string to be diffed.
   * @param text2 - New string to be diffed.
   * @param checklines - If true, use line-level speedup.
   * @param deadline - Time (ms since epoch) when to bail out.
   * @returns Array of diff tuples.
   */
  diff_main(
    text1: string,
    text2: string,
    checklines?: boolean,
    deadline?: number,
  ): Diff[] {
    if (deadline === undefined) {
      if (this.diffTimeout <= 0) {
        deadline = Number.MAX_VALUE;
      } else {
        deadline = Date.now() + this.diffTimeout * 1000;
      }
    }

    if (text1 === text2) {
      if (text1) {
        return [[DIFF_EQUAL, text1]];
      }
      return [];
    }

    // Trim common prefix
    let commonlength = this.diff_commonPrefix(text1, text2);
    const commonprefix = text1.substring(0, commonlength);
    text1 = text1.substring(commonlength);
    text2 = text2.substring(commonlength);

    // Trim common suffix
    commonlength = this.diff_commonSuffix(text1, text2);
    const commonsuffix = text1.substring(text1.length - commonlength);
    text1 = text1.substring(0, text1.length - commonlength);
    text2 = text2.substring(0, text2.length - commonlength);

    // Compute the diff on the middle block
    const diffs = this.diff_compute_(text1, text2, checklines ?? true, deadline);

    // Restore prefix and suffix
    if (commonprefix) {
      diffs.unshift([DIFF_EQUAL, commonprefix]);
    }
    if (commonsuffix) {
      diffs.push([DIFF_EQUAL, commonsuffix]);
    }

    this.diff_cleanupMerge(diffs);
    return diffs;
  }

  /**
   * Compute the diff between two non-empty texts (prefix/suffix already trimmed).
   * @internal
   */
  private diff_compute_(
    text1: string,
    text2: string,
    checklines: boolean,
    deadline: number,
  ): Diff[] {
    if (!text1) {
      return [[DIFF_INSERT, text2]];
    }
    if (!text2) {
      return [[DIFF_DELETE, text1]];
    }

    const longtext = text1.length > text2.length ? text1 : text2;
    const shorttext = text1.length > text2.length ? text2 : text1;
    const i = longtext.indexOf(shorttext);

    if (i !== -1) {
      // Shorter text is inside the longer text
      const diffs: Diff[] = [
        [DIFF_INSERT, longtext.substring(0, i)],
        [DIFF_EQUAL, shorttext],
        [DIFF_INSERT, longtext.substring(i + shorttext.length)],
      ];
      if (text1.length > text2.length) {
        diffs[0][0] = DIFF_DELETE;
        diffs[2][0] = DIFF_DELETE;
      }
      return diffs;
    }

    if (shorttext.length === 1) {
      // Single character strings cannot use line mode
      return [
        [DIFF_DELETE, text1],
        [DIFF_INSERT, text2],
      ];
    }

    // Check for a half-match (common middle)
    const hm = this.diff_halfMatch_(text1, text2);
    if (hm) {
      const [text1_a, text1_b, text2_a, text2_b, mid_common] = hm;
      const diffs_a = this.diff_main(text1_a, text2_a, checklines, deadline);
      const diffs_b = this.diff_main(text1_b, text2_b, checklines, deadline);
      return diffs_a.concat([[DIFF_EQUAL, mid_common]], diffs_b);
    }

    if (checklines && text1.length > 100 && text2.length > 100) {
      return this.diff_lineMode_(text1, text2, deadline);
    }

    return this.diff_bisect_(text1, text2, deadline);
  }

  /**
   * Perform a line-level diff then refine to character level.
   * @internal
   */
  private diff_lineMode_(
    text1: string,
    text2: string,
    deadline: number,
  ): Diff[] {
    // Encode lines as single characters for fast diff
    const { chars1, chars2, lineArray } = this.diff_linesToChars_(text1, text2);

    const diffs = this.diff_main(chars1, chars2, false, deadline);

    // Convert back to text
    this.diff_charsToLines_(diffs, lineArray);

    // Clean up and re-diff overlaps
    this.diff_cleanupSemantic(diffs);

    // Add an empty entry as sentinel
    diffs.push([DIFF_EQUAL, ""]);

    let pointer = 0;
    let countDelete = 0;
    let countInsert = 0;
    let textDelete = "";
    let textInsert = "";

    while (pointer < diffs.length) {
      switch (diffs[pointer][0]) {
        case DIFF_INSERT:
          countInsert++;
          textInsert += diffs[pointer][1];
          break;
        case DIFF_DELETE:
          countDelete++;
          textDelete += diffs[pointer][1];
          break;
        case DIFF_EQUAL:
          if (countDelete >= 1 && countInsert >= 1) {
            // Replace the crude line diffs with character diffs
            const subDiffs = this.diff_main(
              textDelete,
              textInsert,
              false,
              deadline,
            );
            diffs.splice(
              pointer - countDelete - countInsert,
              countDelete + countInsert,
              ...subDiffs,
            );
            pointer = pointer - countDelete - countInsert + subDiffs.length;
          }
          countInsert = 0;
          countDelete = 0;
          textDelete = "";
          textInsert = "";
          break;
      }
      pointer++;
    }
    diffs.pop(); // Remove sentinel

    return diffs;
  }

  /**
   * Myers' diff bisection algorithm.
   * @internal
   */
  private diff_bisect_(
    text1: string,
    text2: string,
    deadline: number,
  ): Diff[] {
    const text1_length = text1.length;
    const text2_length = text2.length;
    const max_d = Math.ceil((text1_length + text2_length) / 2);
    const v_offset = max_d;
    const v_length = 2 * max_d;

    const v1 = new Array(v_length).fill(-1);
    const v2 = new Array(v_length).fill(-1);
    v1[v_offset + 1] = 0;
    v2[v_offset + 1] = 0;

    const delta = text1_length - text2_length;
    const front = delta % 2 !== 0;

    let k1start = 0;
    let k1end = 0;
    let k2start = 0;
    let k2end = 0;

    for (let d = 0; d < max_d; d++) {
      if (Date.now() > deadline) {
        break;
      }

      // Forward path
      for (let k1 = -d + k1start; k1 <= d - k1end; k1 += 2) {
        const k1_offset = v_offset + k1;
        let x1: number;
        if (k1 === -d || (k1 !== d && v1[k1_offset - 1] < v1[k1_offset + 1])) {
          x1 = v1[k1_offset + 1];
        } else {
          x1 = v1[k1_offset - 1] + 1;
        }
        let y1 = x1 - k1;
        while (
          x1 < text1_length &&
          y1 < text2_length &&
          text1.charAt(x1) === text2.charAt(y1)
        ) {
          x1++;
          y1++;
        }
        v1[k1_offset] = x1;
        if (x1 > text1_length) {
          k1end += 2;
        } else if (y1 > text2_length) {
          k1start += 2;
        } else if (front) {
          const k2_offset = v_offset + delta - k1;
          if (k2_offset >= 0 && k2_offset < v_length && v2[k2_offset] !== -1) {
            const x2 = text1_length - v2[k2_offset];
            if (x1 >= x2) {
              return this.diff_bisectSplit_(text1, text2, x1, y1, deadline);
            }
          }
        }
      }

      // Reverse path
      for (let k2 = -d + k2start; k2 <= d - k2end; k2 += 2) {
        const k2_offset = v_offset + k2;
        let x2: number;
        if (k2 === -d || (k2 !== d && v2[k2_offset - 1] < v2[k2_offset + 1])) {
          x2 = v2[k2_offset + 1];
        } else {
          x2 = v2[k2_offset - 1] + 1;
        }
        let y2 = x2 - k2;
        while (
          x2 < text1_length &&
          y2 < text2_length &&
          text1.charAt(text1_length - x2 - 1) ===
            text2.charAt(text2_length - y2 - 1)
        ) {
          x2++;
          y2++;
        }
        v2[k2_offset] = x2;
        if (x2 > text1_length) {
          k2end += 2;
        } else if (y2 > text2_length) {
          k2start += 2;
        } else if (!front) {
          const k1_offset = v_offset + delta - k2;
          if (k1_offset >= 0 && k1_offset < v_length && v1[k1_offset] !== -1) {
            const x1 = v1[k1_offset];
            const y1 = v_offset + x1 - k1_offset;
            x2 = text1_length - x2;
            if (x1 >= x2) {
              return this.diff_bisectSplit_(text1, text2, x1, y1, deadline);
            }
          }
        }
      }
    }

    // Timeout or no match: return a full delete + insert
    return [
      [DIFF_DELETE, text1],
      [DIFF_INSERT, text2],
    ];
  }

  /**
   * Split two texts at the found bisection point and recurse.
   * @internal
   */
  private diff_bisectSplit_(
    text1: string,
    text2: string,
    x: number,
    y: number,
    deadline: number,
  ): Diff[] {
    const text1a = text1.substring(0, x);
    const text2a = text2.substring(0, y);
    const text1b = text1.substring(x);
    const text2b = text2.substring(y);

    const diffs = this.diff_main(text1a, text2a, false, deadline);
    const diffsb = this.diff_main(text1b, text2b, false, deadline);
    return diffs.concat(diffsb);
  }

  /**
   * Encode unique lines as single characters for fast line-level diffing.
   * @internal
   */
  private diff_linesToChars_(
    text1: string,
    text2: string,
  ): { chars1: string; chars2: string; lineArray: string[] } {
    const lineArray: string[] = [];
    const lineHash: Map<string, number> = new Map();

    // \x00 is reserved as a sentinel for the lineArray
    lineArray[0] = "";

    const linesToCharsMunge = (text: string): string => {
      let chars = "";
      let lineStart = 0;
      let lineEnd = -1;
      let lineHashValue: number | undefined;

      while (lineEnd < text.length - 1) {
        lineEnd = text.indexOf("\n", lineStart);
        if (lineEnd === -1) {
          lineEnd = text.length - 1;
        }
        const line = text.substring(lineStart, lineEnd + 1);

        lineHashValue = lineHash.get(line);
        if (lineHashValue !== undefined) {
          chars += String.fromCharCode(lineHashValue);
        } else {
          lineArray.push(line);
          lineHash.set(line, lineArray.length - 1);
          chars += String.fromCharCode(lineArray.length - 1);
        }
        lineStart = lineEnd + 1;
      }
      return chars;
    };

    const chars1 = linesToCharsMunge(text1);
    const chars2 = linesToCharsMunge(text2);
    return { chars1, chars2, lineArray };
  }

  /**
   * Rehydrate char-encoded diffs back into multi-char lines.
   * @internal
   */
  private diff_charsToLines_(diffs: Diff[], lineArray: string[]): void {
    for (const diff of diffs) {
      const chars = diff[1];
      const text: string[] = [];
      for (let j = 0; j < chars.length; j++) {
        text.push(lineArray[chars.charCodeAt(j)]);
      }
      diff[1] = text.join("");
    }
  }

  /**
   * Look for a shared substring that is at least half the length of the longer text.
   * @internal
   */
  private diff_halfMatch_(
    text1: string,
    text2: string,
  ): [string, string, string, string, string] | null {
    if (this.diffTimeout <= 0) {
      return null;
    }
    const longtext = text1.length > text2.length ? text1 : text2;
    const shorttext = text1.length > text2.length ? text2 : text1;
    if (longtext.length < 4 || shorttext.length * 2 < longtext.length) {
      return null;
    }

    const hm1 = this.diff_halfMatchI_(longtext, shorttext, Math.ceil(longtext.length / 4));
    const hm2 = this.diff_halfMatchI_(longtext, shorttext, Math.ceil(longtext.length / 2));

    let hm: string[] | null;
    if (!hm1 && !hm2) {
      return null;
    } else if (!hm2) {
      hm = hm1!;
    } else if (!hm1) {
      hm = hm2;
    } else {
      hm = hm1[4].length > hm2[4].length ? hm1 : hm2;
    }

    let text1_a: string, text1_b: string, text2_a: string, text2_b: string;
    if (text1.length > text2.length) {
      text1_a = hm![0];
      text1_b = hm![1];
      text2_a = hm![2];
      text2_b = hm![3];
    } else {
      text2_a = hm![0];
      text2_b = hm![1];
      text1_a = hm![2];
      text1_b = hm![3];
    }
    return [text1_a, text1_b, text2_a, text2_b, hm![4]];
  }

  /**
   * Check if a substring of shorttext exists within longtext starting at position i.
   * @internal
   */
  private diff_halfMatchI_(
    longtext: string,
    shorttext: string,
    i: number,
  ): string[] | null {
    const seed = longtext.substring(i, i + Math.floor(longtext.length / 4));
    let j = -1;
    let best_common = "";
    let best_longtext_a = "";
    let best_longtext_b = "";
    let best_shorttext_a = "";
    let best_shorttext_b = "";

    while ((j = shorttext.indexOf(seed, j + 1)) !== -1) {
      const prefixLength = this.diff_commonPrefix(
        longtext.substring(i),
        shorttext.substring(j),
      );
      const suffixLength = this.diff_commonSuffix(
        longtext.substring(0, i),
        shorttext.substring(0, j),
      );
      if (best_common.length < suffixLength + prefixLength) {
        best_common =
          shorttext.substring(j - suffixLength, j) +
          shorttext.substring(j, j + prefixLength);
        best_longtext_a = longtext.substring(0, i - suffixLength);
        best_longtext_b = longtext.substring(i + prefixLength);
        best_shorttext_a = shorttext.substring(0, j - suffixLength);
        best_shorttext_b = shorttext.substring(j + prefixLength);
      }
    }

    if (best_common.length * 2 >= longtext.length) {
      return [
        best_longtext_a,
        best_longtext_b,
        best_shorttext_a,
        best_shorttext_b,
        best_common,
      ];
    }
    return null;
  }

  /**
   * Determine the length of the common prefix of two strings using binary search.
   * @param text1 - First string.
   * @param text2 - Second string.
   * @returns Number of common prefix characters.
   */
  diff_commonPrefix(text1: string, text2: string): number {
    if (!text1 || !text2 || text1.charAt(0) !== text2.charAt(0)) {
      return 0;
    }
    let lo = 0;
    let hi = Math.min(text1.length, text2.length);
    let mid = hi;
    let start = 0;
    while (lo < mid) {
      if (text1.substring(start, mid) === text2.substring(start, mid)) {
        lo = mid;
        start = lo;
      } else {
        hi = mid;
      }
      mid = Math.floor((hi - lo) / 2 + lo);
    }
    return mid;
  }

  /**
   * Determine the length of the common suffix of two strings using binary search.
   * @param text1 - First string.
   * @param text2 - Second string.
   * @returns Number of common suffix characters.
   */
  diff_commonSuffix(text1: string, text2: string): number {
    if (
      !text1 ||
      !text2 ||
      text1.charAt(text1.length - 1) !== text2.charAt(text2.length - 1)
    ) {
      return 0;
    }
    let lo = 0;
    let hi = Math.min(text1.length, text2.length);
    let mid = hi;
    let end1 = text1.length;
    let end2 = text2.length;
    while (lo < mid) {
      if (
        text1.substring(end1 - mid, end1 - lo) ===
        text2.substring(end2 - mid, end2 - lo)
      ) {
        lo = mid;
      } else {
        hi = mid;
      }
      mid = Math.floor((hi - lo) / 2 + lo);
    }
    return mid;
  }

  /* ---------------------------------------------------------------- */
  /*  Diff: Cleanup                                                    */
  /* ---------------------------------------------------------------- */

  /**
   * Reduce diffs to a semantically meaningful form by eliminating
   * operationally trivial equalities.
   * @param diffs - Array of diff tuples (modified in place).
   */
  diff_cleanupSemantic(diffs: Diff[]): void {
    let changes = false;
    const equalities: number[] = [];
    let equalitiesLength = 0;
    let lastEquality: string | null = null;
    let pointer = 0;

    let length_insertions1 = 0;
    let length_deletions1 = 0;
    let length_insertions2 = 0;
    let length_deletions2 = 0;

    while (pointer < diffs.length) {
      if (diffs[pointer][0] === DIFF_EQUAL) {
        equalities[equalitiesLength++] = pointer;
        length_insertions1 = length_insertions2;
        length_deletions1 = length_deletions2;
        length_insertions2 = 0;
        length_deletions2 = 0;
        lastEquality = diffs[pointer][1];
      } else {
        if (diffs[pointer][0] === DIFF_INSERT) {
          length_insertions2 += diffs[pointer][1].length;
        } else {
          length_deletions2 += diffs[pointer][1].length;
        }

        if (
          lastEquality &&
          lastEquality.length <=
            Math.max(length_insertions1, length_deletions1) &&
          lastEquality.length <= Math.max(length_insertions2, length_deletions2)
        ) {
          diffs.splice(equalities[equalitiesLength - 1], 0, [
            DIFF_DELETE,
            lastEquality,
          ]);
          diffs[equalities[equalitiesLength - 1] + 1][0] = DIFF_INSERT;
          equalitiesLength--;
          equalitiesLength--;
          pointer =
            equalitiesLength > 0 ? equalities[equalitiesLength - 1] : -1;
          length_insertions1 = 0;
          length_deletions1 = 0;
          length_insertions2 = 0;
          length_deletions2 = 0;
          lastEquality = null;
          changes = true;
        }
      }
      pointer++;
    }

    if (changes) {
      this.diff_cleanupMerge(diffs);
    }
  }

  /**
   * Reduce the number of edits by eliminating operationally trivial equalities.
   * @param diffs - Array of diff tuples (modified in place).
   */
  diff_cleanupEfficiency(diffs: Diff[]): void {
    let changes = false;
    const equalities: number[] = [];
    let equalitiesLength = 0;
    let lastEquality: string | null = null;
    let pointer = 0;

    let pre_ins = false;
    let pre_del = false;
    let post_ins = false;
    let post_del = false;

    while (pointer < diffs.length) {
      if (diffs[pointer][0] === DIFF_EQUAL) {
        if (
          diffs[pointer][1].length < this.diffEditCost &&
          (post_ins || post_del)
        ) {
          equalities[equalitiesLength++] = pointer;
          pre_ins = post_ins;
          pre_del = post_del;
          lastEquality = diffs[pointer][1];
        } else {
          equalitiesLength = 0;
          lastEquality = null;
        }
        post_ins = false;
        post_del = false;
      } else {
        if (diffs[pointer][0] === DIFF_DELETE) {
          post_del = true;
        } else {
          post_ins = true;
        }

        if (
          lastEquality &&
          ((pre_ins && pre_del && post_ins && post_del) ||
            (lastEquality.length < this.diffEditCost / 2 &&
              (pre_ins ? 1 : 0) +
                (pre_del ? 1 : 0) +
                (post_ins ? 1 : 0) +
                (post_del ? 1 : 0) ===
                3))
        ) {
          diffs.splice(equalities[equalitiesLength - 1], 0, [
            DIFF_DELETE,
            lastEquality,
          ]);
          diffs[equalities[equalitiesLength - 1] + 1][0] = DIFF_INSERT;
          equalitiesLength--;
          lastEquality = null;
          if (pre_ins && pre_del) {
            post_ins = true;
            post_del = true;
            equalitiesLength = 0;
          } else {
            equalitiesLength--;
            pointer =
              equalitiesLength > 0 ? equalities[equalitiesLength - 1] : -1;
            post_ins = false;
            post_del = false;
          }
          changes = true;
        }
      }
      pointer++;
    }

    if (changes) {
      this.diff_cleanupMerge(diffs);
    }
  }

  /**
   * Reorder and merge like diff tuples, merging equalities.
   * Any non-empty edit following an equality is merged into that equality.
   * @param diffs - Array of diff tuples (modified in place).
   */
  diff_cleanupMerge(diffs: Diff[]): void {
    diffs.push([DIFF_EQUAL, ""]);
    let pointer = 0;
    let countDelete = 0;
    let countInsert = 0;
    let textDelete = "";
    let textInsert = "";

    while (pointer < diffs.length) {
      switch (diffs[pointer][0]) {
        case DIFF_INSERT:
          countInsert++;
          textInsert += diffs[pointer][1];
          pointer++;
          break;
        case DIFF_DELETE:
          countDelete++;
          textDelete += diffs[pointer][1];
          pointer++;
          break;
        case DIFF_EQUAL:
          if (countDelete + countInsert > 1) {
            if (countDelete !== 0 && countInsert !== 0) {
              // Factor out any common prefixes
              let commonlength = this.diff_commonPrefix(textInsert, textDelete);
              if (commonlength !== 0) {
                const idx = pointer - countDelete - countInsert - 1;
                if (idx >= 0 && diffs[idx][0] === DIFF_EQUAL) {
                  diffs[idx][1] += textInsert.substring(0, commonlength);
                } else {
                  diffs.splice(0, 0, [
                    DIFF_EQUAL,
                    textInsert.substring(0, commonlength),
                  ]);
                  pointer++;
                }
                textInsert = textInsert.substring(commonlength);
                textDelete = textDelete.substring(commonlength);
              }
              // Factor out any common suffixes
              commonlength = this.diff_commonSuffix(textInsert, textDelete);
              if (commonlength !== 0) {
                diffs[pointer][1] =
                  textInsert.substring(textInsert.length - commonlength) +
                  diffs[pointer][1];
                textInsert = textInsert.substring(
                  0,
                  textInsert.length - commonlength,
                );
                textDelete = textDelete.substring(
                  0,
                  textDelete.length - commonlength,
                );
              }
            }
            // Remove empty entries
            const count = countDelete + countInsert;
            if (countDelete === 0) {
              diffs.splice(pointer - count, count, [DIFF_INSERT, textInsert]);
            } else if (countInsert === 0) {
              diffs.splice(pointer - count, count, [DIFF_DELETE, textDelete]);
            } else {
              diffs.splice(
                pointer - count,
                count,
                [DIFF_DELETE, textDelete],
                [DIFF_INSERT, textInsert],
              );
            }
            pointer = pointer - count + (countDelete ? 1 : 0) + (countInsert ? 1 : 0) + 1;
          } else if (pointer !== 0 && diffs[pointer - 1][0] === DIFF_EQUAL) {
            // Merge this equality with the previous one
            diffs[pointer - 1][1] += diffs[pointer][1];
            diffs.splice(pointer, 1);
          } else {
            pointer++;
          }
          countInsert = 0;
          countDelete = 0;
          textDelete = "";
          textInsert = "";
          break;
      }
    }
    if (diffs[diffs.length - 1][1] === "") {
      diffs.pop();
    }

    // Second pass: look for single edits surrounded on both sides by equalities
    // that can be shifted sideways to eliminate an equality
    let changes = false;
    pointer = 1;
    while (pointer < diffs.length - 1) {
      if (
        diffs[pointer - 1][0] === DIFF_EQUAL &&
        diffs[pointer + 1][0] === DIFF_EQUAL
      ) {
        if (
          diffs[pointer][1].substring(
            diffs[pointer][1].length - diffs[pointer - 1][1].length,
          ) === diffs[pointer - 1][1]
        ) {
          diffs[pointer][1] =
            diffs[pointer - 1][1] +
            diffs[pointer][1].substring(
              0,
              diffs[pointer][1].length - diffs[pointer - 1][1].length,
            );
          diffs[pointer + 1][1] = diffs[pointer - 1][1] + diffs[pointer + 1][1];
          diffs.splice(pointer - 1, 1);
          changes = true;
        } else if (
          diffs[pointer][1].substring(0, diffs[pointer + 1][1].length) ===
          diffs[pointer + 1][1]
        ) {
          diffs[pointer - 1][1] += diffs[pointer + 1][1];
          diffs[pointer][1] =
            diffs[pointer][1].substring(diffs[pointer + 1][1].length) +
            diffs[pointer + 1][1];
          diffs.splice(pointer + 1, 1);
          changes = true;
        }
      }
      pointer++;
    }
    if (changes) {
      this.diff_cleanupMerge(diffs);
    }
  }

  /* ---------------------------------------------------------------- */
  /*  Match: Fuzzy matching                                            */
  /* ---------------------------------------------------------------- */

  /**
   * Locate the best instance of a pattern in the text near a given location.
   * @param text - The text to search.
   * @param pattern - The pattern to search for.
   * @param loc - The location to search around.
   * @returns Best match index or -1.
   */
  match_main(text: string, pattern: string, loc: number): number {
    loc = Math.max(0, Math.min(loc, text.length));
    if (text === pattern) {
      return 0;
    } else if (!text.length) {
      return -1;
    } else if (text.substring(loc, loc + pattern.length) === pattern) {
      return loc;
    } else {
      return this.match_bitap_(text, pattern, loc);
    }
  }

  /**
   * Bitap fuzzy matching algorithm.
   * @internal
   */
  private match_bitap_(text: string, pattern: string, loc: number): number {
    if (pattern.length > this.matchMaxBits) {
      // For patterns longer than our max bits, we fall back to exact matching only
      const index = text.indexOf(pattern, loc);
      return index;
    }

    // Initialize the alphabet
    const s: Record<string, number> = {};
    for (let i = 0; i < pattern.length; i++) {
      s[pattern.charAt(i)] = (s[pattern.charAt(i)] || 0) | (1 << (pattern.length - i - 1));
    }

    let score_threshold = this.matchThreshold;
    let best_loc = text.indexOf(pattern, loc);
    if (best_loc !== -1) {
      score_threshold = Math.min(
        this.match_bitapScore_(0, best_loc, loc, pattern),
        score_threshold,
      );
      // Reverse check
      best_loc = text.lastIndexOf(pattern, loc + pattern.length);
      if (best_loc !== -1) {
        score_threshold = Math.min(
          this.match_bitapScore_(0, best_loc, loc, pattern),
          score_threshold,
        );
      }
    }

    const matchmask = 1 << (pattern.length - 1);
    best_loc = -1;

    let bin_max = pattern.length + text.length;
    let last_rd: number[] | undefined;

    for (let d = 0; d < pattern.length; d++) {
      let start = 0;
      let finish = bin_max;
      let rd: number[];

      // Bisect to find the furthest match location
      while (start < finish) {
        const mid = Math.floor((finish - start) / 2) + start;
        if (
          this.match_bitapScore_(d + 1, loc + mid, loc, pattern) <=
          score_threshold
        ) {
          start = mid + 1;
        } else {
          finish = mid;
        }
      }
      bin_max = finish;

      let begin = Math.max(1, loc - finish + 1);
      const end = Math.min(loc + finish, text.length) + pattern.length;

      rd = new Array(end + 2);
      rd[end + 1] = (1 << d) - 1;

      for (let j = end; j >= begin; j--) {
        const charMatch = s[text.charAt(j - 1)] || 0;
        if (d === 0) {
          rd[j] = ((rd[j + 1] << 1) | 1) & charMatch;
        } else {
          rd[j] =
            (((rd[j + 1] << 1) | 1) & charMatch) |
            (((last_rd![j + 1] | last_rd![j]) << 1) | 1) |
            last_rd![j + 1];
        }
        if (rd[j] & matchmask) {
          const score = this.match_bitapScore_(d, j - 1, loc, pattern);
          if (score <= score_threshold) {
            score_threshold = score;
            best_loc = j - 1;
            if (best_loc > loc) {
              begin = Math.max(1, 2 * loc - best_loc);
            } else {
              break;
            }
          }
        }
      }

      if (
        this.match_bitapScore_(d + 1, loc, loc, pattern) > score_threshold
      ) {
        break;
      }
      last_rd = rd;
    }
    return best_loc;
  }

  /**
   * Compute the score for a match at the given distance from the expected location.
   * @internal
   */
  private match_bitapScore_(
    e: number,
    x: number,
    loc: number,
    pattern: string,
  ): number {
    const accuracy = e / pattern.length;
    const proximity = Math.abs(loc - x);
    if (!this.matchDistance) {
      return proximity ? 1.0 : accuracy;
    }
    return accuracy + proximity / this.matchDistance;
  }

  /* ---------------------------------------------------------------- */
  /*  Patch: Creation                                                   */
  /* ---------------------------------------------------------------- */

  /**
   * Create a list of patches from a source text and a set of diffs.
   * @param text - The original text (before diffs were applied).
   * @param diffs - Array of diff tuples for text → new text.
   * @returns Array of Patch objects.
   */
  patch_make(text: string, diffs: Diff[]): Patch[] {
    const patches: Patch[] = [];
    if (diffs.length === 0) {
      return patches;
    }

    let patch = new Patch();
    let patchDiffLength = 0;
    let char_count1 = 0;
    let char_count2 = 0;
    let prepatch_text = text;
    let postpatch_text = text;

    for (let x = 0; x < diffs.length; x++) {
      const diff_type = diffs[x][0];
      const diff_text = diffs[x][1];

      if (patchDiffLength === 0 && diff_type !== DIFF_EQUAL) {
        patch.start1 = char_count1;
        patch.start2 = char_count2;
      }

      switch (diff_type) {
        case DIFF_INSERT:
          patch.diffs[patchDiffLength++] = diffs[x];
          patch.length2 += diff_text.length;
          postpatch_text =
            postpatch_text.substring(0, char_count2) +
            diff_text +
            postpatch_text.substring(char_count2);
          break;
        case DIFF_DELETE:
          patch.length1 += diff_text.length;
          patch.diffs[patchDiffLength++] = diffs[x];
          postpatch_text =
            postpatch_text.substring(0, char_count2) +
            postpatch_text.substring(char_count2 + diff_text.length);
          break;
        case DIFF_EQUAL:
          if (
            diff_text.length <= 2 * this.patchMargin &&
            patchDiffLength !== 0 &&
            diffs.length !== x + 1
          ) {
            // Small equality inside a patch
            patch.diffs[patchDiffLength++] = diffs[x];
            patch.length1 += diff_text.length;
            patch.length2 += diff_text.length;
          } else if (
            diff_text.length >= 2 * this.patchMargin &&
            patchDiffLength !== 0
          ) {
            // Large equality: emit the current patch
            this.patch_addContext(patch, prepatch_text);
            patches.push(patch);
            patch = new Patch();
            patchDiffLength = 0;
            prepatch_text = postpatch_text;
            char_count1 = char_count2;
          }
          break;
      }

      if (diff_type !== DIFF_INSERT) {
        char_count1 += diff_text.length;
      }
      if (diff_type !== DIFF_DELETE) {
        char_count2 += diff_text.length;
      }
    }

    // Pick up the leftover patch
    if (patchDiffLength) {
      this.patch_addContext(patch, prepatch_text);
      patches.push(patch);
    }

    return patches;
  }

  /**
   * Add context lines to a patch to improve match accuracy during application.
   * @param patch - The patch to add context to.
   * @param text - The source text.
   */
  patch_addContext(patch: Patch, text: string): void {
    if (text.length === 0) return;

    let pattern = text.substring(patch.start2, patch.start2 + patch.length1);
    let padding = 0;

    // Increase context until the pattern is unique or we hit the margin limit
    while (
      text.indexOf(pattern) !== text.lastIndexOf(pattern) &&
      pattern.length < this.matchMaxBits - this.patchMargin - this.patchMargin
    ) {
      padding += this.patchMargin;
      pattern = text.substring(
        Math.max(0, patch.start2 - padding),
        patch.start2 + patch.length1 + padding,
      );
    }
    // Add one more chunk for safety
    padding += this.patchMargin;

    // Add prefix context
    const prefix = text.substring(
      Math.max(0, patch.start2 - padding),
      patch.start2,
    );
    if (prefix) {
      patch.diffs.unshift([DIFF_EQUAL, prefix]);
      patch.start1 -= prefix.length;
      patch.start2 -= prefix.length;
      patch.length1 += prefix.length;
      patch.length2 += prefix.length;
    }

    // Add suffix context
    const suffix = text.substring(
      patch.start2 + patch.length1,
      patch.start2 + patch.length1 + padding,
    );
    if (suffix) {
      patch.diffs.push([DIFF_EQUAL, suffix]);
      patch.length1 += suffix.length;
      patch.length2 += suffix.length;
    }
  }

  /* ---------------------------------------------------------------- */
  /*  Patch: Application                                               */
  /* ---------------------------------------------------------------- */

  /**
   * Apply a list of patches to text. Returns the patched text and an array
   * of booleans indicating which patches were successfully applied.
   * @param patches - Array of Patch objects.
   * @param text - The text to patch.
   * @returns Tuple of [patched_text, results_array].
   */
  patch_apply(patches: Patch[], text: string): [string, boolean[]] {
    if (patches.length === 0) {
      return [text, []];
    }

    // Deep-copy patches to avoid mutating originals
    patches = this.patch_deepCopy(patches);

    const nullPadding = this.patch_addPadding(patches);
    text = nullPadding + text + nullPadding;

    const results: boolean[] = new Array(patches.length).fill(false);

    let delta = 0;
    for (let x = 0; x < patches.length; x++) {
      const expected_loc = patches[x].start2 + delta;
      const text1 = this.diff_text1(patches[x].diffs);
      let start_loc: number;
      let end_loc = -1;

      if (text1.length > this.matchMaxBits) {
        // Long patch: look for the start and end separately
        start_loc = this.match_main(
          text,
          text1.substring(0, this.matchMaxBits),
          expected_loc,
        );
        if (start_loc !== -1) {
          end_loc = this.match_main(
            text,
            text1.substring(text1.length - this.matchMaxBits),
            expected_loc + text1.length - this.matchMaxBits,
          );
          if (end_loc === -1 || start_loc >= end_loc) {
            start_loc = -1;
          }
        }
      } else {
        start_loc = this.match_main(text, text1, expected_loc);
      }

      if (start_loc === -1) {
        // No match found
        results[x] = false;
        delta -= patches[x].length2 - patches[x].length1;
      } else {
        results[x] = true;
        delta = start_loc - expected_loc;
        let text2: string;

        if (end_loc === -1) {
          text2 = text.substring(start_loc, start_loc + text1.length);
        } else {
          text2 = text.substring(start_loc, end_loc + this.matchMaxBits);
        }

        if (text1 === text2) {
          // Perfect match: splice in the full replacement
          text =
            text.substring(0, start_loc) +
            this.diff_text2(patches[x].diffs) +
            text.substring(start_loc + text1.length);
        } else {
          // Imperfect match: use diff to find character-level changes
          const diffs = this.diff_main(text1, text2, false);
          if (
            text1.length > this.matchMaxBits &&
            this.diff_levenshtein(diffs) / text1.length >
              1 - this.matchThreshold
          ) {
            // Poor quality match
            results[x] = false;
          } else {
            this.diff_cleanupSemanticLossless(diffs);
            let index1 = 0;
            let index2: number;
            for (const diff of patches[x].diffs) {
              if (diff[0] !== DIFF_EQUAL) {
                index2 = this.diff_xIndex(diffs, index1);
              }
              if (diff[0] === DIFF_INSERT) {
                text =
                  text.substring(0, start_loc + index2!) +
                  diff[1] +
                  text.substring(start_loc + index2!);
              } else if (diff[0] === DIFF_DELETE) {
                const endIndex = this.diff_xIndex(
                  diffs,
                  index1 + diff[1].length,
                );
                text =
                  text.substring(0, start_loc + index2!) +
                  text.substring(start_loc + endIndex);
              }
              if (diff[0] !== DIFF_DELETE) {
                index1 += diff[1].length;
              }
            }
          }
        }
      }
    }

    // Strip padding
    text = text.substring(
      nullPadding.length,
      text.length - nullPadding.length,
    );
    return [text, results];
  }

  /**
   * Add null-padding around patches to handle edge effects.
   * @internal
   */
  private patch_addPadding(patches: Patch[]): string {
    const paddingLength = this.patchMargin;
    let nullPadding = "";
    for (let x = 1; x <= paddingLength; x++) {
      nullPadding += String.fromCharCode(x);
    }

    // Bump all patches forward
    for (const patch of patches) {
      patch.start1 += paddingLength;
      patch.start2 += paddingLength;
    }

    // Add leading context to the first patch
    let patch = patches[0];
    let diffs = patch.diffs;
    if (diffs.length === 0 || diffs[0][0] !== DIFF_EQUAL) {
      diffs.unshift([DIFF_EQUAL, nullPadding]);
      patch.start1 -= paddingLength;
      patch.start2 -= paddingLength;
      patch.length1 += paddingLength;
      patch.length2 += paddingLength;
    } else if (paddingLength > diffs[0][1].length) {
      const extraLength = paddingLength - diffs[0][1].length;
      diffs[0][1] = nullPadding.substring(diffs[0][1].length) + diffs[0][1];
      patch.start1 -= extraLength;
      patch.start2 -= extraLength;
      patch.length1 += extraLength;
      patch.length2 += extraLength;
    }

    // Add trailing context to the last patch
    patch = patches[patches.length - 1];
    diffs = patch.diffs;
    if (diffs.length === 0 || diffs[diffs.length - 1][0] !== DIFF_EQUAL) {
      diffs.push([DIFF_EQUAL, nullPadding]);
      patch.length1 += paddingLength;
      patch.length2 += paddingLength;
    } else if (paddingLength > diffs[diffs.length - 1][1].length) {
      const extraLength =
        paddingLength - diffs[diffs.length - 1][1].length;
      diffs[diffs.length - 1][1] += nullPadding.substring(
        0,
        extraLength,
      );
      patch.length1 += extraLength;
      patch.length2 += extraLength;
    }

    return nullPadding;
  }

  /**
   * Deep copy an array of Patch objects.
   * @internal
   */
  private patch_deepCopy(patches: Patch[]): Patch[] {
    const patchesCopy: Patch[] = [];
    for (const aPatch of patches) {
      const patchCopy = new Patch();
      patchCopy.diffs = [];
      for (const aDiff of aPatch.diffs) {
        patchCopy.diffs.push([aDiff[0], aDiff[1]]);
      }
      patchCopy.start1 = aPatch.start1;
      patchCopy.start2 = aPatch.start2;
      patchCopy.length1 = aPatch.length1;
      patchCopy.length2 = aPatch.length2;
      patchesCopy.push(patchCopy);
    }
    return patchesCopy;
  }

  /* ---------------------------------------------------------------- */
  /*  Diff: Utility methods                                            */
  /* ---------------------------------------------------------------- */

  /**
   * Compute the source text (text1) from a set of diffs.
   * @internal
   */
  private diff_text1(diffs: Diff[]): string {
    const parts: string[] = [];
    for (const [op, text] of diffs) {
      if (op !== DIFF_INSERT) {
        parts.push(text);
      }
    }
    return parts.join("");
  }

  /**
   * Compute the destination text (text2) from a set of diffs.
   * @internal
   */
  private diff_text2(diffs: Diff[]): string {
    const parts: string[] = [];
    for (const [op, text] of diffs) {
      if (op !== DIFF_DELETE) {
        parts.push(text);
      }
    }
    return parts.join("");
  }

  /**
   * Compute Levenshtein distance from a set of diffs.
   * @internal
   */
  private diff_levenshtein(diffs: Diff[]): number {
    let levenshtein = 0;
    let insertions = 0;
    let deletions = 0;
    for (const [op, text] of diffs) {
      switch (op) {
        case DIFF_INSERT:
          insertions += text.length;
          break;
        case DIFF_DELETE:
          deletions += text.length;
          break;
        case DIFF_EQUAL:
          levenshtein += Math.max(insertions, deletions);
          insertions = 0;
          deletions = 0;
          break;
      }
    }
    levenshtein += Math.max(insertions, deletions);
    return levenshtein;
  }

  /**
   * Given a set of diffs, find the corresponding position in text2 for a
   * position in text1.
   * @internal
   */
  private diff_xIndex(diffs: Diff[], loc: number): number {
    let chars1 = 0;
    let chars2 = 0;
    let last_chars1 = 0;
    let last_chars2 = 0;

    for (let x = 0; x < diffs.length; x++) {
      if (diffs[x][0] !== DIFF_INSERT) {
        chars1 += diffs[x][1].length;
      }
      if (diffs[x][0] !== DIFF_DELETE) {
        chars2 += diffs[x][1].length;
      }
      if (chars1 > loc) {
        break;
      }
      last_chars1 = chars1;
      last_chars2 = chars2;
    }

    if (diffs.length !== chars1 && diffs[diffs.length - 1]?.[0] === DIFF_DELETE) {
      return last_chars2;
    }
    return last_chars2 + (loc - last_chars1);
  }

  /**
   * Lossless cleanup: shift diffs to align with token boundaries.
   * @internal
   */
  private diff_cleanupSemanticLossless(diffs: Diff[]): void {
    let pointer = 1;
    while (pointer < diffs.length - 1) {
      if (
        diffs[pointer - 1][0] === DIFF_EQUAL &&
        diffs[pointer + 1][0] === DIFF_EQUAL
      ) {
        let equality1 = diffs[pointer - 1][1];
        let edit = diffs[pointer][1];
        let equality2 = diffs[pointer + 1][1];

        // Find the "best" score for repositioning the edit
        const commonOffset = this.diff_commonSuffix(equality1, edit);
        if (commonOffset) {
          const commonString = edit.substring(edit.length - commonOffset);
          equality1 = equality1.substring(0, equality1.length - commonOffset);
          edit = commonString + edit.substring(0, edit.length - commonOffset);
          equality2 = commonString + equality2;
        }

        let bestEquality1 = equality1;
        let bestEdit = edit;
        let bestEquality2 = equality2;
        let bestScore =
          this.diff_cleanupSemanticScore_(equality1, edit) +
          this.diff_cleanupSemanticScore_(edit, equality2);

        while (edit.charAt(0) === equality2.charAt(0)) {
          equality1 += edit.charAt(0);
          edit = edit.substring(1) + equality2.charAt(0);
          equality2 = equality2.substring(1);
          const score =
            this.diff_cleanupSemanticScore_(equality1, edit) +
            this.diff_cleanupSemanticScore_(edit, equality2);
          if (score >= bestScore) {
            bestScore = score;
            bestEquality1 = equality1;
            bestEdit = edit;
            bestEquality2 = equality2;
          }
        }

        if (diffs[pointer - 1][1] !== bestEquality1) {
          if (bestEquality1) {
            diffs[pointer - 1][1] = bestEquality1;
          } else {
            diffs.splice(pointer - 1, 1);
            pointer--;
          }
          diffs[pointer][1] = bestEdit;
          if (bestEquality2) {
            diffs[pointer + 1][1] = bestEquality2;
          } else {
            diffs.splice(pointer + 1, 1);
            pointer--;
          }
        }
      }
      pointer++;
    }
  }

  /**
   * Score function for semantic cleanup.
   * @internal
   */
  private diff_cleanupSemanticScore_(one: string, two: string): number {
    if (!one || !two) {
      return 6; // Blank lines are worth a lot
    }
    const char1 = one.charAt(one.length - 1);
    const char2 = two.charAt(0);
    const nonAlphaNumeric1 = char1.match(/[^a-zA-Z0-9]/);
    const nonAlphaNumeric2 = char2.match(/[^a-zA-Z0-9]/);
    const whitespace1 = nonAlphaNumeric1 && char1.match(/\s/);
    const whitespace2 = nonAlphaNumeric2 && char2.match(/\s/);
    const lineBreak1 = whitespace1 && char1.match(/[\r\n]/);
    const lineBreak2 = whitespace2 && char2.match(/[\r\n]/);
    const blankLine1 = lineBreak1 && one.match(/\n\r?\n$/);
    const blankLine2 = lineBreak2 && two.match(/^\r?\n\r?\n/);

    if (blankLine1 || blankLine2) return 5;
    if (lineBreak1 || lineBreak2) return 4;
    if (nonAlphaNumeric1 && !whitespace1 && whitespace2) return 3;
    if (whitespace1 || whitespace2) return 2;
    if (nonAlphaNumeric1 || nonAlphaNumeric2) return 1;
    return 0;
  }
}

/* ------------------------------------------------------------------ */
/*  Three-way merge                                                    */
/* ------------------------------------------------------------------ */

/**
 * Perform a three-way merge of concurrent text changes.
 *
 * Given a common base version, a locally modified version, and a remotely
 * modified version, produce a merged result by computing the local changes
 * as patches and applying them to the remote text.
 *
 * @param base - The common ancestor text.
 * @param local - The locally modified text.
 * @param remote - The remotely modified text.
 * @returns The merged text.
 */
export function threeWayMerge(
  base: string,
  local: string,
  remote: string,
): string {
  const dmp = new DiffMatchPatch();

  // Compute diffs between base and local
  const diffs = dmp.diff_main(base, local, true);

  // Clean up for efficiency
  dmp.diff_cleanupSemantic(diffs);
  dmp.diff_cleanupEfficiency(diffs);

  // Create patches from base → local
  const patches = dmp.patch_make(base, diffs);

  // Apply patches to remote text
  const [merged] = dmp.patch_apply(patches, remote);

  return merged;
}
