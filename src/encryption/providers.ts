/**
 * @module encryption/providers
 *
 * Concrete encryption provider implementations for each protocol version.
 *
 * ## Version overview
 *
 * | Version | Path encryption     | Content encryption | Key derivation        |
 * |---------|---------------------|--------------------|------------------------|
 * | 0       | AES-GCM deterministic IV (SHA-256 of path) | AES-GCM random IV | scrypt → SHA-256      |
 * | 2 / 3   | AES-SIV             | AES-GCM (HKDF-derived key) | scrypt → HKDF |
 *
 * The encryption key is always derived from the user's password + vault salt
 * via scrypt.  The key hash is computed differently per version and is sent to
 * the server to validate access.
 */

import { webcrypto } from "node:crypto";
import {
  stringToBuffer,
  bufferToHex,
  hexToBuffer,
  toArrayBuffer,
} from "../utils/encoding.js";
import {
  sha256,
  aesGcmEncrypt,
  aesGcmDecrypt,
  importAesGcmKey,
} from "../utils/crypto.js";
import { AesSiv } from "./aes-siv.js";
import type { EncryptionProvider, EncryptionVersion } from "./types.js";

const subtle = webcrypto.subtle;

/* ====================================================================
 * Scrypt key derivation
 * ==================================================================== */

/** scrypt parameters (matching the Obsidian desktop client). */
const SCRYPT_N = 32768;
const SCRYPT_R = 8;
const SCRYPT_P = 1;
const SCRYPT_DKLEN = 32;

/**
 * Derive a 32-byte raw key from `password` and `salt` using scrypt.
 * Both arguments are NFKC-normalised before hashing.
 */
export async function deriveKey(
  password: string,
  salt: string,
): Promise<ArrayBuffer> {
  const normPass = password.normalize("NFKC");
  const normSalt = salt.normalize("NFKC");

  // Use Node.js native scrypt for performance
  const crypto = await import("node:crypto");
  const buf: Buffer = await new Promise((resolve, reject) => {
    crypto.scrypt(
      Buffer.from(normPass, "utf8"),
      Buffer.from(normSalt, "utf8"),
      SCRYPT_DKLEN,
      {
        N: SCRYPT_N,
        r: SCRYPT_R,
        p: SCRYPT_P,
        maxmem: 128 * SCRYPT_N * SCRYPT_R * 2,
      },
      (err, key) => {
        if (err) reject(err);
        else resolve(key);
      },
    );
  });

  return toArrayBuffer(buf);
}

/* ====================================================================
 * Key hash computation (sent to server for validation)
 * ==================================================================== */

/**
 * Compute the key hash for a given encryption version.
 *
 * - V0: `hex( SHA-256( rawKey ) )`
 * - V2/V3: `hex( HKDF( rawKey, salt, "ObsidianKeyHash" ) )` exported as raw AES-CBC key
 */
export async function computeKeyHash(
  rawKey: ArrayBuffer,
  salt: string,
  version: EncryptionVersion,
): Promise<string> {
  switch (version) {
    case 0:
      return bufferToHex(await sha256(rawKey));

    case 2:
    case 3: {
      const hkdfKey = await subtle.importKey("raw", rawKey, "HKDF", false, [
        "deriveKey",
      ]);
      const derived = await subtle.deriveKey(
        {
          name: "HKDF",
          salt: stringToBuffer(salt),
          info: stringToBuffer("ObsidianKeyHash"),
          hash: "SHA-256",
        },
        hkdfKey,
        { name: "AES-CBC", length: 256 },
        true,
        ["encrypt"],
      );
      return bufferToHex(await subtle.exportKey("raw", derived));
    }

    default:
      throw new Error("Encryption version not supported");
  }
}

/* ====================================================================
 * V0 – Legacy AES-GCM encryption
 * ==================================================================== */

/**
 * Encryption V0 (legacy).
 *
 * - **Paths** are encrypted with AES-GCM using a *deterministic* IV
 *   computed as `SHA-256(utf8(path))[0..12]`.
 * - **Content** is encrypted with AES-GCM using a random 12-byte IV.
 * - The key hash is `SHA-256(rawKey)`.
 */
class EncryptionV0 implements EncryptionProvider {
  readonly encryptionVersion = 0 as const;

  private constructor(
    public readonly keyHash: string,
    private readonly cryptoKey: CryptoKey,
  ) {}

  static async init(rawKey: ArrayBuffer): Promise<EncryptionV0> {
    if (rawKey.byteLength !== 32) {
      throw new Error("Invalid encryption key");
    }
    const keyHash = bufferToHex(await sha256(rawKey));
    const cryptoKey = await importAesGcmKey(rawKey);
    return new EncryptionV0(keyHash, cryptoKey);
  }

  async deterministicEncodeStr(path: string): Promise<string> {
    const plaintext = stringToBuffer(path);
    const hash = await sha256(plaintext);
    const iv = new Uint8Array(hash, 0, 12);
    const ct = await aesGcmEncrypt(plaintext, this.cryptoKey, iv);
    return bufferToHex(ct);
  }

  async deterministicDecodeStr(encoded: string): Promise<string> {
    const buf = hexToBuffer(encoded);
    const { bufferToString } = await import("../utils/encoding.js");
    return bufferToString(await aesGcmDecrypt(buf, this.cryptoKey));
  }

  async encrypt(data: ArrayBuffer): Promise<ArrayBuffer> {
    return aesGcmEncrypt(data, this.cryptoKey);
  }

  async decrypt(data: ArrayBuffer): Promise<ArrayBuffer> {
    return aesGcmDecrypt(data, this.cryptoKey);
  }
}

/* ====================================================================
 * V2/V3 – AES-SIV paths + HKDF-derived AES-GCM content
 * ==================================================================== */

/**
 * Encryption V2/V3.
 *
 * - **Paths** are encrypted with AES-SIV (deterministic authenticated
 *   encryption).  The two AES-SIV sub-keys are HKDF-derived with info
 *   strings `"ObsidianAesSivEnc"` and `"ObsidianAesSivMac"`.
 * - **Content** is encrypted with AES-GCM using a separate HKDF-derived
 *   key (info `"ObsidianAesGcm"`, salt = empty).
 * - The key hash is HKDF-derived with info `"ObsidianKeyHash"`.
 */
class EncryptionV2V3 implements EncryptionProvider {
  readonly encryptionVersion: 2 | 3;

  private constructor(
    public readonly keyHash: string,
    private readonly cryptoKey: CryptoKey,
    private readonly siv: AesSiv,
    version: 2 | 3,
  ) {
    if (version !== 2 && version !== 3) {
      throw new Error("Invalid encryption version");
    }
    this.encryptionVersion = version;
  }

  static async init(
    rawKey: ArrayBuffer,
    salt: string,
    version: 2 | 3,
  ): Promise<EncryptionV2V3> {
    if (rawKey.byteLength !== 32) {
      throw new Error("Invalid encryption key");
    }

    const hkdfBaseKey = await subtle.importKey("raw", rawKey, "HKDF", false, [
      "deriveKey",
    ]);
    const saltBuf = new TextEncoder().encode(salt);

    // 1. Key hash for server authentication
    const keyHashKey = await subtle.deriveKey(
      {
        name: "HKDF",
        salt: saltBuf,
        info: stringToBuffer("ObsidianKeyHash"),
        hash: "SHA-256",
      },
      hkdfBaseKey,
      { name: "AES-CBC", length: 256 },
      true,
      ["encrypt"],
    );
    const keyHash = bufferToHex(await subtle.exportKey("raw", keyHashKey));

    // 2. AES-SIV for path encryption
    const siv = await AesSiv.importKey(hkdfBaseKey, saltBuf);

    // 3. AES-GCM key for content encryption (salt = empty, info = "ObsidianAesGcm")
    const gcmKey = await subtle.deriveKey(
      {
        name: "HKDF",
        salt: new Uint8Array(),
        info: stringToBuffer("ObsidianAesGcm"),
        hash: "SHA-256",
      },
      hkdfBaseKey,
      { name: "AES-GCM", length: 256 },
      false,
      ["encrypt", "decrypt"],
    );

    return new EncryptionV2V3(keyHash, gcmKey, siv, version);
  }

  async deterministicEncodeStr(path: string): Promise<string> {
    const plaintext = stringToBuffer(path);
    const ct = await this.siv.seal(new Uint8Array(plaintext));
    return bufferToHex(toArrayBuffer(ct));
  }

  async deterministicDecodeStr(encoded: string): Promise<string> {
    const buf = hexToBuffer(encoded);
    const pt = await this.siv.open(new Uint8Array(buf));
    const { bufferToString } = await import("../utils/encoding.js");
    return bufferToString(toArrayBuffer(pt));
  }

  async encrypt(data: ArrayBuffer): Promise<ArrayBuffer> {
    return aesGcmEncrypt(data, this.cryptoKey);
  }

  async decrypt(data: ArrayBuffer): Promise<ArrayBuffer> {
    return aesGcmDecrypt(data, this.cryptoKey);
  }
}

/* ====================================================================
 * Factory
 * ==================================================================== */

/**
 * Create the appropriate encryption provider for the given version.
 *
 * @param version  Encryption version (0, 2, or 3).
 * @param rawKey   The 32-byte scrypt-derived key.
 * @param salt     The vault salt string (only used for V2/V3).
 */
export async function createEncryptionProvider(
  version: EncryptionVersion,
  rawKey: ArrayBuffer,
  salt: string,
): Promise<EncryptionProvider> {
  switch (version) {
    case 0:
      return EncryptionV0.init(rawKey);
    case 2:
    case 3:
      return EncryptionV2V3.init(rawKey, salt, version);
    default:
      throw new Error("Encryption version not supported");
  }
}
