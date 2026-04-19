/**
 * @module encryption/aes-siv
 *
 * Pure-JavaScript AES-SIV (RFC 5297) implementation used for deterministic
 * path encryption in V2/V3.
 *
 * AES-SIV provides *deterministic authenticated encryption*: the same
 * plaintext always produces the same ciphertext.  This is essential so the
 * server can index files by their encrypted path without knowing the
 * cleartext.
 *
 * Internally SIV is built from two sub-primitives:
 *   1. AES-CMAC  (for the S2V MAC function)
 *   2. AES-CTR   (for the encryption step)
 *
 * The key material is derived from the user password via HKDF so the
 * CMAC key and CTR key are distinct.
 */
/** Error thrown on AES-SIV authentication failure. */
export declare class AesSivError extends Error {
    constructor(message: string);
}
/**
 * AES-SIV authenticated encryption.
 *
 * Usage:
 * ```ts
 * const siv = await AesSiv.importKey(hkdfBaseKey, salt);
 * const ct  = await siv.seal(plaintext);
 * const pt  = await siv.open(ct);
 * ```
 */
export declare class AesSiv {
    private readonly cmacFactory;
    private readonly ctr;
    private constructor();
    /**
     * Derive the two AES-SIV sub-keys from an HKDF base key and salt.
     */
    static importKey(hkdfKey: CryptoKey, salt: Uint8Array): Promise<AesSiv>;
    /**
     * Encrypt `plaintext` producing `[ SIV tag (16 bytes) ][ ciphertext ]`.
     */
    seal(plaintext: Uint8Array): Promise<Uint8Array>;
    /**
     * Decrypt and verify a ciphertext produced by {@link seal}.
     */
    open(ciphertext: Uint8Array): Promise<Uint8Array>;
    /**
     * S2V (String-to-Vector) – the PRF used by AES-SIV.
     * Produces a 16-byte synthetic IV from the plaintext.
     */
    private s2v;
}
//# sourceMappingURL=aes-siv.d.ts.map