"use strict";
/**
 * @module utils/paths
 *
 * Path manipulation helpers that mirror the conventions used inside the
 * Obsidian vault file system.  Paths are always stored with forward
 * slashes, NFC-normalised, and without leading/trailing separators.
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.INSENSITIVE_FS = exports.ALL_KNOWN_EXTS = exports.BASE_EXTS = exports.CANVAS_EXTS = exports.MARKDOWN_EXTS = exports.PDF_EXTS = exports.VIDEO_EXTS = exports.AUDIO_EXTS = exports.IMAGE_EXTS = void 0;
exports.normalizeSpaces = normalizeSpaces;
exports.normalizePath = normalizePath;
exports.sanitizePath = sanitizePath;
exports.basename = basename;
exports.dirname = dirname;
exports.extname = extname;
exports.basenameNoExt = basenameNoExt;
exports.pathNoExt = pathNoExt;
exports.isHiddenPath = isHiddenPath;
exports.isKnownExtension = isKnownExtension;
exports.isValidFilename = isValidFilename;
exports.validateFilename = validateFilename;
exports.sanitizeFilename = sanitizeFilename;
/* ------------------------------------------------------------------ */
/*  Basic path helpers                                                */
/* ------------------------------------------------------------------ */
/**
 * Non-breaking-space and narrow-no-break-space → regular space.
 */
const NBSP_RE = /\u00A0|\u202F/g;
function normalizeSpaces(str) {
    return str.replace(NBSP_RE, " ");
}
/**
 * Normalise a vault-relative path: collapse slashes, strip leading/trailing
 * separators and apply NFC normalisation.
 */
function normalizePath(path) {
    let p = path.replace(/([\\/])+/g, "/").replace(/(^\/+|\/+$)/g, "");
    if (p === "")
        p = "/";
    return normalizeSpaces(p).normalize("NFC");
}
/**
 * Strip duplicate slashes and leading/trailing `/`.
 */
function sanitizePath(path) {
    let p = path.replace(/([\\/])+/g, "/").replace(/(^\/+|\/+$)/g, "");
    if (p === "")
        p = "/";
    return p;
}
/**
 * Return the file name portion of `path` (everything after the last `/`).
 */
function basename(path) {
    const i = path.lastIndexOf("/");
    return i === -1 ? path : path.slice(i + 1);
}
/**
 * Return the parent directory of `path` (everything before the last `/`).
 * Returns `""` for top-level files.
 */
function dirname(path) {
    const i = path.lastIndexOf("/");
    return i === -1 ? "" : path.slice(0, i);
}
/**
 * Return the file extension (without the dot) in lower case, or `""`.
 */
function extname(path) {
    const name = basename(path);
    const dot = name.lastIndexOf(".");
    if (dot === -1 || dot === name.length - 1 || dot === 0)
        return "";
    return name.substr(dot + 1).toLowerCase();
}
/**
 * Return the file name without its extension.
 */
function basenameNoExt(path) {
    const name = basename(path);
    const dot = name.lastIndexOf(".");
    if (dot === -1 || dot === name.length - 1 || dot === 0)
        return name;
    return name.substr(0, dot);
}
/**
 * Return the path without its extension (keeps the directory portion).
 */
function pathNoExt(path) {
    const dot = path.lastIndexOf(".");
    if (dot === -1 || dot === path.length - 1 || dot === 0)
        return path;
    return path.substr(0, dot);
}
/**
 * Returns `true` if *any* component of `path` starts with `.` (hidden file).
 */
function isHiddenPath(path) {
    let p = path;
    while (p) {
        if (basename(p).startsWith("."))
            return true;
        p = dirname(p);
    }
    return false;
}
/* ------------------------------------------------------------------ */
/*  File type classification                                          */
/* ------------------------------------------------------------------ */
exports.IMAGE_EXTS = [
    "bmp",
    "png",
    "jpg",
    "jpeg",
    "gif",
    "svg",
    "webp",
    "avif",
];
exports.AUDIO_EXTS = [
    "mp3",
    "wav",
    "m4a",
    "3gp",
    "flac",
    "ogg",
    "oga",
    "opus",
];
exports.VIDEO_EXTS = ["mp4", "webm", "ogv", "mov", "mkv"];
exports.PDF_EXTS = ["pdf"];
exports.MARKDOWN_EXTS = ["md"];
exports.CANVAS_EXTS = ["canvas"];
exports.BASE_EXTS = ["base"];
/** All extensions that Obsidian considers a recognised file type. */
exports.ALL_KNOWN_EXTS = [
    ...exports.IMAGE_EXTS,
    ...exports.AUDIO_EXTS,
    ...exports.VIDEO_EXTS,
    ...exports.PDF_EXTS,
    ...exports.MARKDOWN_EXTS,
    ...exports.CANVAS_EXTS,
];
/**
 * Returns `true` when the extension is one Obsidian will open natively.
 */
function isKnownExtension(ext) {
    return exports.ALL_KNOWN_EXTS.includes(ext);
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
function escapeRegex(s) {
    return s.replace(/[.?*+^$[\]\\(){}|-]/g, "\\$&");
}
/**
 * Returns `true` when the vault-relative path contains no illegal chars.
 */
function isValidFilename(path) {
    try {
        validateFilename(path);
        return true;
    }
    catch {
        return false;
    }
}
/**
 * Throws if the path contains characters illegal on the current OS.
 */
function validateFilename(path) {
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
        throw new Error("File name cannot contain any of the following characters: " +
            ILLEGAL_CHARS.split("").join("\u00A0"));
    }
}
/**
 * Replace illegal characters in `name` with `replacement`.
 */
function sanitizeFilename(name, replacement = "_") {
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
exports.INSENSITIVE_FS = IS_MAC || IS_WIN;
//# sourceMappingURL=paths.js.map