/**
 * @module encryption/types
 *
 * Shared types for the encryption subsystem.  Three encryption versions exist:
 *
 * - **V0** (legacy) – AES-GCM with deterministic IV for paths.
 * - **V2** – AES-SIV for paths, HKDF-derived AES-GCM for content.
 * - **V3** – Same as V2 (protocol-level differences only).
 */

/** Supported encryption version numbers. */
export type EncryptionVersion = 0 | 2 | 3;

/**
 * An encryption provider can deterministically encode vault-relative paths
 * (used by the server for file identification) and encrypt/decrypt arbitrary
 * file content.
 */
export interface EncryptionProvider {
  /** The version of the encryption protocol. */
  readonly encryptionVersion: EncryptionVersion;

  /**
   * The key hash that is sent to the server during authentication.
   * For V0 this is SHA-256(raw_key); for V2/V3 it is HKDF-derived.
   */
  readonly keyHash: string;

  /**
   * Deterministically encode a vault-relative path into a hex string.
   * Same path always produces the same output so the server can match files.
   */
  deterministicEncodeStr(path: string): Promise<string>;

  /**
   * Reverse of {@link deterministicEncodeStr}.
   */
  deterministicDecodeStr(encoded: string): Promise<string>;

  /**
   * Encrypt file content with a random IV.
   *
   * @returns `[ IV (12 bytes) ][ ciphertext + auth tag ]`
   */
  encrypt(data: ArrayBuffer): Promise<ArrayBuffer>;

  /**
   * Decrypt content produced by {@link encrypt}.
   */
  decrypt(data: ArrayBuffer): Promise<ArrayBuffer>;
}
