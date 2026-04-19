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
import type { EncryptionProvider, EncryptionVersion } from "./types.js";
/**
 * Derive a 32-byte raw key from `password` and `salt` using scrypt.
 * Both arguments are NFKC-normalised before hashing.
 */
export declare function deriveKey(password: string, salt: string): Promise<ArrayBuffer>;
/**
 * Compute the key hash for a given encryption version.
 *
 * - V0: `hex( SHA-256( rawKey ) )`
 * - V2/V3: `hex( HKDF( rawKey, salt, "ObsidianKeyHash" ) )` exported as raw AES-CBC key
 */
export declare function computeKeyHash(rawKey: ArrayBuffer, salt: string, version: EncryptionVersion): Promise<string>;
/**
 * Create the appropriate encryption provider for the given version.
 *
 * @param version  Encryption version (0, 2, or 3).
 * @param rawKey   The 32-byte scrypt-derived key.
 * @param salt     The vault salt string (only used for V2/V3).
 */
export declare function createEncryptionProvider(version: EncryptionVersion, rawKey: ArrayBuffer, salt: string): Promise<EncryptionProvider>;
//# sourceMappingURL=providers.d.ts.map