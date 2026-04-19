/**
 * @module utils/paths
 *
 * Path manipulation helpers that mirror the conventions used inside the
 * Obsidian vault file system.  Paths are always stored with forward
 * slashes, NFC-normalised, and without leading/trailing separators.
 */

/* ------------------------------------------------------------------ */
/*  Basic path helpers                                                */
/* ------------------------------------------------------------------ */

/**
 * Non-breaking-space and narrow-no-break-space → regular space.
 */
const NBSP_RE = /\u00A0|\u202F/g;

export function normalizeSpaces(str: string): string {
  return str.replace(NBSP_RE, " ");
}

/**
 * Normalise a vault-relative path: collapse slashes, strip leading/trailing
 * separators and apply NFC normalisation.
 */
export function normalizePath(path: string): string {
  let p = path.replace(/([\\/])+/g, "/").replace(/(^\/+|\/+$)/g, "");
  if (p === "") p = "/";
  return normalizeSpaces(p).normalize("NFC");
}

/**
 * Strip duplicate slashes and leading/trailing `/`.
 */
export function sanitizePath(path: string): string {
  let p = path.replace(/([\\/])+/g, "/").replace(/(^\/+|\/+$)/g, "");
  if (p === "") p = "/";
  return p;
}

/**
 * Return the file name portion of `path` (everything after the last `/`).
 */
export function basename(path: string): string {
  const i = path.lastIndexOf("/");
  return i === -1 ? path : path.slice(i + 1);
}

/**
 * Return the parent directory of `path` (everything before the last `/`).
 * Returns `""` for top-level files.
 */
export function dirname(path: string): string {
  const i = path.lastIndexOf("/");
  return i === -1 ? "" : path.slice(0, i);
}

/**
 * Return the file extension (without the dot) in lower case, or `""`.
 */
export function extname(path: string): string {
  const name = basename(path);
  const dot = name.lastIndexOf(".");
  if (dot === -1 || dot === name.length - 1 || dot === 0) return "";
  return name.substr(dot + 1).toLowerCase();
}

/**
 * Return the file name without its extension.
 */
export function basenameNoExt(path: string): string {
  const name = basename(path);
  const dot = name.lastIndexOf(".");
  if (dot === -1 || dot === name.length - 1 || dot === 0) return name;
  return name.substr(0, dot);
}

/**
 * Return the path without its extension (keeps the directory portion).
 */
export function pathNoExt(path: string): string {
  const dot = path.lastIndexOf(".");
  if (dot === -1 || dot === path.length - 1 || dot === 0) return path;
  return path.substr(0, dot);
}

/**
 * Returns `true` if *any* component of `path` starts with `.` (hidden file).
 */
export function isHiddenPath(path: string): boolean {
  let p: string = path;
  while (p) {
    if (basename(p).startsWith(".")) return true;
    p = dirname(p);
  }
  return false;
}

/* ------------------------------------------------------------------ */
/*  File type classification                                          */
/* ------------------------------------------------------------------ */

export const IMAGE_EXTS = [
  "bmp",
  "png",
  "jpg",
  "jpeg",
  "gif",
  "svg",
  "webp",
  "avif",
];
export const AUDIO_EXTS = [
  "mp3",
  "wav",
  "m4a",
  "3gp",
  "flac",
  "ogg",
  "oga",
  "opus",
];
export const VIDEO_EXTS = ["mp4", "webm", "ogv", "mov", "mkv"];
export const PDF_EXTS = ["pdf"];
export const MARKDOWN_EXTS = ["md"];
export const CANVAS_EXTS = ["canvas"];
export const BASE_EXTS = ["base"];

/** All extensions that Obsidian considers a recognised file type. */
export const ALL_KNOWN_EXTS = [
  ...IMAGE_EXTS,
  ...AUDIO_EXTS,
  ...VIDEO_EXTS,
  ...PDF_EXTS,
  ...MARKDOWN_EXTS,
  ...CANVAS_EXTS,
];

/**
 * Returns `true` when the extension is one Obsidian will open natively.
 */
export function isKnownExtension(ext: string): boolean {
  return ALL_KNOWN_EXTS.includes(ext);
}

/* ------------------------------------------------------------------ */
/*  Filename validation (platform-aware)                               */
/* ------------------------------------------------------------------ */

const IS_WIN = process.platform === "win32";
const IS_MAC = process.platform === "darwin";
const IS_ANDROID = false; // Node CLI never runs on Android

const ILLEGAL_ON_ANDROID = '*?<>"';
const ILLEGAL_ON_OTHER = "\\/:" + ILLEGAL_ON_ANDROID;
const ILLEGAL_ON_WIN = '*"\\/<>:|?';

const ILLEGAL_CHARS = IS_WIN ? ILLEGAL_ON_WIN : ILLEGAL_ON_OTHER;
const ILLEGAL_RE = new RegExp("[" + escapeRegex(ILLEGAL_CHARS) + "]");
const RESERVED_WIN = /^(CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])$/i;

function escapeRegex(s: string): string {
  return s.replace(/[.?*+^$[\]\\(){}|-]/g, "\\$&");
}

/**
 * Returns `true` when the vault-relative path contains no illegal chars.
 */
export function isValidFilename(path: string): boolean {
  try {
    validateFilename(path);
    return true;
  } catch {
    return false;
  }
}

/**
 * Throws if the path contains characters illegal on the current OS.
 */
export function validateFilename(path: string): void {
  if (IS_WIN) {
    const last = path.charAt(path.length - 1);
    if (last === "." || last === " ") {
      throw new Error("File names cannot end with a dot or a space.");
    }
    const name = basenameNoExt(path);
    if (RESERVED_WIN.test(name)) {
      throw new Error("File name is forbidden: " + name);
    }
  }
  if (path.split("/").some((seg) => ILLEGAL_RE.test(seg))) {
    throw new Error(
      "File name cannot contain any of the following characters: " +
        ILLEGAL_CHARS.split("").join("\u00A0"),
    );
  }
}

/**
 * Replace illegal characters in `name` with `replacement`.
 */
export function sanitizeFilename(
  name: string,
  replacement = "_",
): string {
  let result = name.trim();
  if (result) {
    result = name.replace(ILLEGAL_RE, replacement);
    if (replacement.length === 1) {
      const esc = escapeRegex(replacement);
      result = result.replace(new RegExp(`${esc}{2,}`, "g"), replacement);
    }
  }
  return result;
}

/** Case-insensitive file system (macOS / Windows). */
export const INSENSITIVE_FS = IS_MAC || IS_WIN;
