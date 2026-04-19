/**
 * @module utils/paths
 *
 * Path manipulation helpers that mirror the conventions used inside the
 * Obsidian vault file system.  Paths are always stored with forward
 * slashes, NFC-normalised, and without leading/trailing separators.
 */
export declare function normalizeSpaces(str: string): string;
/**
 * Normalise a vault-relative path: collapse slashes, strip leading/trailing
 * separators and apply NFC normalisation.
 */
export declare function normalizePath(path: string): string;
/**
 * Strip duplicate slashes and leading/trailing `/`.
 */
export declare function sanitizePath(path: string): string;
/**
 * Return the file name portion of `path` (everything after the last `/`).
 */
export declare function basename(path: string): string;
/**
 * Return the parent directory of `path` (everything before the last `/`).
 * Returns `""` for top-level files.
 */
export declare function dirname(path: string): string;
/**
 * Return the file extension (without the dot) in lower case, or `""`.
 */
export declare function extname(path: string): string;
/**
 * Return the file name without its extension.
 */
export declare function basenameNoExt(path: string): string;
/**
 * Return the path without its extension (keeps the directory portion).
 */
export declare function pathNoExt(path: string): string;
/**
 * Returns `true` if *any* component of `path` starts with `.` (hidden file).
 */
export declare function isHiddenPath(path: string): boolean;
export declare const IMAGE_EXTS: string[];
export declare const AUDIO_EXTS: string[];
export declare const VIDEO_EXTS: string[];
export declare const PDF_EXTS: string[];
export declare const MARKDOWN_EXTS: string[];
export declare const CANVAS_EXTS: string[];
export declare const BASE_EXTS: string[];
/** All extensions that Obsidian considers a recognised file type. */
export declare const ALL_KNOWN_EXTS: string[];
/**
 * Returns `true` when the extension is one Obsidian will open natively.
 */
export declare function isKnownExtension(ext: string): boolean;
/**
 * Returns `true` when the vault-relative path contains no illegal chars.
 */
export declare function isValidFilename(path: string): boolean;
/**
 * Throws if the path contains characters illegal on the current OS.
 */
export declare function validateFilename(path: string): void;
/**
 * Replace illegal characters in `name` with `replacement`.
 */
export declare function sanitizeFilename(name: string, replacement?: string): string;
/** Case-insensitive file system (macOS / Windows). */
export declare const INSENSITIVE_FS: boolean;
//# sourceMappingURL=paths.d.ts.map