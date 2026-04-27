/**
 * @module utils/crypto
 *
 * Thin wrappers around the Web Crypto API (via `node:crypto`) for SHA-256
 * hashing and AES-GCM encrypt/decrypt.  Every function works with raw
 * ArrayBuffers so the caller can choose the serialisation format.
 */

import { webcrypto } from "node:crypto";

const subtle = webcrypto.subtle;

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
export async function sha256(data: ArrayBuffer): Promise<ArrayBuffer> {
  return subtle.digest("SHA-256", new Uint8Array(data));
}

/**
 * Convenience: SHA-256 of `data`, returned as a hex string.
 */
export async function sha256Hex(data: ArrayBuffer): Promise<string> {
  const { bufferToHex } = await import("./encoding.js");
  return bufferToHex(await sha256(data));
}

/* ------------------------------------------------------------------ */
/*  AES-GCM                                                           */
/* ------------------------------------------------------------------ */

/**
 * Import a raw 256-bit key for AES-GCM encrypt + decrypt.
 */
export async function importAesGcmKey(
  raw: ArrayBuffer,
): Promise<CryptoKey> {
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
export async function aesGcmEncrypt(
  plaintext: ArrayBuffer,
  key: CryptoKey,
  iv?: Uint8Array,
): Promise<ArrayBuffer> {
  const actualIv =
    iv ?? (webcrypto.getRandomValues(new Uint8Array(IV_LENGTH)));
  const ct = await subtle.encrypt(
    { name: AES_GCM, iv: actualIv as Uint8Array<ArrayBuffer> },
    key,
    plaintext,
  );
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
export async function aesGcmDecrypt(
  data: ArrayBuffer,
  key: CryptoKey,
): Promise<ArrayBuffer> {
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

export { subtle, webcrypto };
