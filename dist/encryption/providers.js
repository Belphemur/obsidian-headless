"use strict";
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
Object.defineProperty(exports, "__esModule", { value: true });
exports.deriveKey = deriveKey;
exports.computeKeyHash = computeKeyHash;
exports.createEncryptionProvider = createEncryptionProvider;
const node_crypto_1 = require("node:crypto");
const encoding_js_1 = require("../utils/encoding.js");
const crypto_js_1 = require("../utils/crypto.js");
const aes_siv_js_1 = require("./aes-siv.js");
const subtle = node_crypto_1.webcrypto.subtle;
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
async function deriveKey(password, salt) {
    const normPass = password.normalize("NFKC");
    const normSalt = salt.normalize("NFKC");
    // Use Node.js native scrypt for performance
    const crypto = await import("node:crypto");
    const buf = await new Promise((resolve, reject) => {
        crypto.scrypt(Buffer.from(normPass, "utf8"), Buffer.from(normSalt, "utf8"), SCRYPT_DKLEN, {
            N: SCRYPT_N,
            r: SCRYPT_R,
            p: SCRYPT_P,
            maxmem: 128 * SCRYPT_N * SCRYPT_R * 2,
        }, (err, key) => {
            if (err)
                reject(err);
            else
                resolve(key);
        });
    });
    return (0, encoding_js_1.toArrayBuffer)(buf);
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
async function computeKeyHash(rawKey, salt, version) {
    switch (version) {
        case 0:
            return (0, encoding_js_1.bufferToHex)(await (0, crypto_js_1.sha256)(rawKey));
        case 2:
        case 3: {
            const hkdfKey = await subtle.importKey("raw", rawKey, "HKDF", false, [
                "deriveKey",
            ]);
            const derived = await subtle.deriveKey({
                name: "HKDF",
                salt: (0, encoding_js_1.stringToBuffer)(salt),
                info: (0, encoding_js_1.stringToBuffer)("ObsidianKeyHash"),
                hash: "SHA-256",
            }, hkdfKey, { name: "AES-CBC", length: 256 }, true, ["encrypt"]);
            return (0, encoding_js_1.bufferToHex)(await subtle.exportKey("raw", derived));
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
class EncryptionV0 {
    keyHash;
    cryptoKey;
    encryptionVersion = 0;
    constructor(keyHash, cryptoKey) {
        this.keyHash = keyHash;
        this.cryptoKey = cryptoKey;
    }
    static async init(rawKey) {
        if (rawKey.byteLength !== 32) {
            throw new Error("Invalid encryption key");
        }
        const keyHash = (0, encoding_js_1.bufferToHex)(await (0, crypto_js_1.sha256)(rawKey));
        const cryptoKey = await (0, crypto_js_1.importAesGcmKey)(rawKey);
        return new EncryptionV0(keyHash, cryptoKey);
    }
    async deterministicEncodeStr(path) {
        const plaintext = (0, encoding_js_1.stringToBuffer)(path);
        const hash = await (0, crypto_js_1.sha256)(plaintext);
        const iv = new Uint8Array(hash, 0, 12);
        const ct = await (0, crypto_js_1.aesGcmEncrypt)(plaintext, this.cryptoKey, iv);
        return (0, encoding_js_1.bufferToHex)(ct);
    }
    async deterministicDecodeStr(encoded) {
        const buf = (0, encoding_js_1.hexToBuffer)(encoded);
        const { bufferToString } = await import("../utils/encoding.js");
        return bufferToString(await (0, crypto_js_1.aesGcmDecrypt)(buf, this.cryptoKey));
    }
    async encrypt(data) {
        return (0, crypto_js_1.aesGcmEncrypt)(data, this.cryptoKey);
    }
    async decrypt(data) {
        return (0, crypto_js_1.aesGcmDecrypt)(data, this.cryptoKey);
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
class EncryptionV2V3 {
    keyHash;
    cryptoKey;
    siv;
    encryptionVersion;
    constructor(keyHash, cryptoKey, siv, version) {
        this.keyHash = keyHash;
        this.cryptoKey = cryptoKey;
        this.siv = siv;
        if (version !== 2 && version !== 3) {
            throw new Error("Invalid encryption version");
        }
        this.encryptionVersion = version;
    }
    static async init(rawKey, salt, version) {
        if (rawKey.byteLength !== 32) {
            throw new Error("Invalid encryption key");
        }
        const hkdfBaseKey = await subtle.importKey("raw", rawKey, "HKDF", false, [
            "deriveKey",
        ]);
        const saltBuf = new TextEncoder().encode(salt);
        // 1. Key hash for server authentication
        const keyHashKey = await subtle.deriveKey({
            name: "HKDF",
            salt: saltBuf,
            info: (0, encoding_js_1.stringToBuffer)("ObsidianKeyHash"),
            hash: "SHA-256",
        }, hkdfBaseKey, { name: "AES-CBC", length: 256 }, true, ["encrypt"]);
        const keyHash = (0, encoding_js_1.bufferToHex)(await subtle.exportKey("raw", keyHashKey));
        // 2. AES-SIV for path encryption
        const siv = await aes_siv_js_1.AesSiv.importKey(hkdfBaseKey, saltBuf);
        // 3. AES-GCM key for content encryption (salt = empty, info = "ObsidianAesGcm")
        const gcmKey = await subtle.deriveKey({
            name: "HKDF",
            salt: new Uint8Array(),
            info: (0, encoding_js_1.stringToBuffer)("ObsidianAesGcm"),
            hash: "SHA-256",
        }, hkdfBaseKey, { name: "AES-GCM", length: 256 }, false, ["encrypt", "decrypt"]);
        return new EncryptionV2V3(keyHash, gcmKey, siv, version);
    }
    async deterministicEncodeStr(path) {
        const plaintext = (0, encoding_js_1.stringToBuffer)(path);
        const ct = await this.siv.seal(new Uint8Array(plaintext));
        return (0, encoding_js_1.bufferToHex)((0, encoding_js_1.toArrayBuffer)(ct));
    }
    async deterministicDecodeStr(encoded) {
        const buf = (0, encoding_js_1.hexToBuffer)(encoded);
        const pt = await this.siv.open(new Uint8Array(buf));
        const { bufferToString } = await import("../utils/encoding.js");
        return bufferToString((0, encoding_js_1.toArrayBuffer)(pt));
    }
    async encrypt(data) {
        return (0, crypto_js_1.aesGcmEncrypt)(data, this.cryptoKey);
    }
    async decrypt(data) {
        return (0, crypto_js_1.aesGcmDecrypt)(data, this.cryptoKey);
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
async function createEncryptionProvider(version, rawKey, salt) {
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
//# sourceMappingURL=providers.js.map