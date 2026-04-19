"use strict";
/**
 * @module utils/crypto
 *
 * Thin wrappers around the Web Crypto API (via `node:crypto`) for SHA-256
 * hashing and AES-GCM encrypt/decrypt.  Every function works with raw
 * ArrayBuffers so the caller can choose the serialisation format.
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.webcrypto = exports.subtle = void 0;
exports.sha256 = sha256;
exports.sha256Hex = sha256Hex;
exports.importAesGcmKey = importAesGcmKey;
exports.aesGcmEncrypt = aesGcmEncrypt;
exports.aesGcmDecrypt = aesGcmDecrypt;
const node_crypto_1 = require("node:crypto");
Object.defineProperty(exports, "webcrypto", { enumerable: true, get: function () { return node_crypto_1.webcrypto; } });
const subtle = node_crypto_1.webcrypto.subtle;
exports.subtle = subtle;
/** The algorithm identifier used for AES-GCM operations. */
const AES_GCM = "AES-GCM";
/** Standard IV length for AES-GCM (96 bits / 12 bytes). */
const IV_LENGTH = 12;
/* ------------------------------------------------------------------ */
/*  Hashing                                                           */
/* ------------------------------------------------------------------ */
/**
 * Compute the SHA-256 digest of `data`.
 *
 * @returns The raw 32-byte hash as an ArrayBuffer.
 */
async function sha256(data) {
    return subtle.digest("SHA-256", new Uint8Array(data));
}
/**
 * Convenience: SHA-256 of `data`, returned as a hex string.
 */
async function sha256Hex(data) {
    const { bufferToHex } = await import("./encoding.js");
    return bufferToHex(await sha256(data));
}
/* ------------------------------------------------------------------ */
/*  AES-GCM                                                           */
/* ------------------------------------------------------------------ */
/**
 * Import a raw 256-bit key for AES-GCM encrypt + decrypt.
 */
async function importAesGcmKey(raw) {
    return subtle.importKey("raw", raw, { name: AES_GCM }, false, [
        "encrypt",
        "decrypt",
    ]);
}
/**
 * Encrypt `plaintext` with AES-256-GCM.
 *
 * Format of the returned buffer:
 * ```
 * [ IV (12 bytes) ][ ciphertext + auth tag ]
 * ```
 *
 * If `iv` is provided the caller takes responsibility for uniqueness
 * (used for deterministic path encryption in V0).  Otherwise a random
 * IV is generated.
 */
async function aesGcmEncrypt(plaintext, key, iv) {
    const actualIv = iv ?? (node_crypto_1.webcrypto.getRandomValues(new Uint8Array(IV_LENGTH)));
    const ct = await subtle.encrypt({ name: AES_GCM, iv: actualIv }, key, plaintext);
    // Prepend IV to ciphertext
    const out = new Uint8Array(actualIv.length + ct.byteLength);
    out.set(actualIv);
    out.set(new Uint8Array(ct), actualIv.length);
    return out.buffer;
}
/**
 * Decrypt an AES-256-GCM payload produced by {@link aesGcmEncrypt}.
 *
 * Expects the first 12 bytes to be the IV.
 */
async function aesGcmDecrypt(data, key) {
    if (data.byteLength < IV_LENGTH) {
        throw new Error("Encrypted data is bad");
    }
    // An "empty" ciphertext (IV-only) decodes to an empty buffer.
    if (data.byteLength === IV_LENGTH) {
        return new ArrayBuffer(0);
    }
    const iv = new Uint8Array(data, 0, IV_LENGTH);
    const ct = new Uint8Array(data, IV_LENGTH);
    return subtle.decrypt({ name: AES_GCM, iv }, key, ct);
}
//# sourceMappingURL=crypto.js.map