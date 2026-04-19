/**
 * @module utils/crypto
 *
 * Thin wrappers around the Web Crypto API (via `node:crypto`) for SHA-256
 * hashing and AES-GCM encrypt/decrypt.  Every function works with raw
 * ArrayBuffers so the caller can choose the serialisation format.
 */
import { webcrypto } from "node:crypto";
declare const subtle: webcrypto.SubtleCrypto;
/**
 * Compute the SHA-256 digest of `data`.
 *
 * @returns The raw 32-byte hash as an ArrayBuffer.
 */
export declare function sha256(data: ArrayBuffer): Promise<ArrayBuffer>;
/**
 * Convenience: SHA-256 of `data`, returned as a hex string.
 */
export declare function sha256Hex(data: ArrayBuffer): Promise<string>;
/**
 * Import a raw 256-bit key for AES-GCM encrypt + decrypt.
 */
export declare function importAesGcmKey(raw: ArrayBuffer): Promise<CryptoKey>;
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
export declare function aesGcmEncrypt(plaintext: ArrayBuffer, key: CryptoKey, iv?: Uint8Array): Promise<ArrayBuffer>;
/**
 * Decrypt an AES-256-GCM payload produced by {@link aesGcmEncrypt}.
 *
 * Expects the first 12 bytes to be the IV.
 */
export declare function aesGcmDecrypt(data: ArrayBuffer, key: CryptoKey): Promise<ArrayBuffer>;
export { subtle, webcrypto };
//# sourceMappingURL=crypto.d.ts.map