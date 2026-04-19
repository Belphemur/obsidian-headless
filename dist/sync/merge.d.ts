/**
 * @module sync/merge
 *
 * Three-way merge engine using the diff-match-patch algorithm.
 * Implements a functional subset of Google's diff-match-patch library
 * sufficient for merging concurrent text edits in Obsidian vaults.
 */
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
export declare function threeWayMerge(base: string, local: string, remote: string): string;
//# sourceMappingURL=merge.d.ts.map