"use strict";
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
Object.defineProperty(exports, "__esModule", { value: true });
exports.AesSiv = exports.AesSivError = void 0;
const node_crypto_1 = require("node:crypto");
const subtle = node_crypto_1.webcrypto.subtle;
/* ====================================================================
 * GF(2^128) block arithmetic (for CMAC doubling)
 * ==================================================================== */
/** A 128-bit (16-byte) block that supports GF(2^128) operations. */
class Block {
    static SIZE = 16;
    /** The reduction polynomial constant R = 0x87 (for x^128 + x^7 + x^2 + x + 1). */
    static R = 0x87;
    data;
    constructor() {
        this.data = new Uint8Array(Block.SIZE);
    }
    /** Zero every byte. */
    clear() {
        this.data.fill(0);
    }
    /**
     * In-place doubling in GF(2^128).
     * `this = this << 1` modulo the reduction polynomial.
     */
    dbl() {
        const d = this.data;
        let carry = 0;
        for (let i = Block.SIZE - 1; i >= 0; i--) {
            const tmp = d[i];
            d[i] = ((tmp << 1) | carry) & 0xff;
            carry = tmp >>> 7;
        }
        // If the MSB was set before shifting, XOR with R in the last byte.
        d[Block.SIZE - 1] ^= select(carry, Block.R, 0);
    }
}
/* ====================================================================
 * AES-CTR cipher  (used as the E() step in SIV)
 * ==================================================================== */
class AesCtr {
    key;
    constructor(key) {
        this.key = key;
    }
    static async importKey(hkdfKey, salt) {
        const derived = await subtle.deriveKey({
            name: "HKDF",
            salt: salt,
            info: new TextEncoder().encode("ObsidianAesSivEnc"),
            hash: "SHA-256",
        }, hkdfKey, { name: "AES-CTR", length: 256 }, false, ["encrypt"]);
        return new AesCtr(derived);
    }
    /**
     * Encrypt/decrypt `data` using AES-CTR with the given `counter` block.
     */
    async encryptCtr(counter, data) {
        const ct = await subtle.encrypt({ name: "AES-CTR", counter: counter, length: 128 }, this.key, data);
        return new Uint8Array(ct);
    }
}
/* ====================================================================
 * AES-CBC MAC  (building block for CMAC)
 * ==================================================================== */
class AesCbcMac {
    key;
    constructor(key) {
        this.key = key;
    }
    static async importKey(hkdfKey, salt) {
        const derived = await subtle.deriveKey({
            name: "HKDF",
            salt: salt,
            info: new TextEncoder().encode("ObsidianAesSivMac"),
            hash: "SHA-256",
        }, hkdfKey, { name: "AES-CBC", length: 256 }, false, ["encrypt"]);
        return new AesCbcMac(derived);
    }
    /**
     * AES-CBC encrypt a single block (16 bytes) with IV = 0.
     * Effectively `AES_K(block)` – a one-block MAC.
     */
    async encryptBlock(block) {
        const iv = new Uint8Array(16);
        const ct = await subtle.encrypt({ name: "AES-CBC", iv }, this.key, block);
        // AES-CBC returns block + padding block.  We only need the first 16 bytes.
        return new Uint8Array(ct, 0, 16);
    }
}
/* ====================================================================
 * CMAC  (NIST SP 800-38B)
 * ==================================================================== */
class Cmac {
    cipher;
    _subkey1;
    _subkey2;
    _buffer = new Block();
    _bufferPos = 0;
    _finished = false;
    constructor(cipher) {
        this.cipher = cipher;
    }
    /**
     * Import CMAC keys.  Returns a *factory* that produces fresh Cmac
     * instances (needed because each CMAC invocation has its own state).
     */
    static async importKey(hkdfKey, salt) {
        const cipher = await AesCbcMac.importKey(hkdfKey, salt);
        // Derive CMAC sub-keys K1 and K2 from L = AES_K(0^128).
        const L = new Block();
        const lEnc = await cipher.encryptBlock(L.data);
        const k1Block = new Block();
        k1Block.data.set(lEnc);
        k1Block.dbl();
        const k1 = new Uint8Array(k1Block.data);
        const k2Block = new Block();
        k2Block.data.set(k1);
        k2Block.dbl();
        const k2 = new Uint8Array(k2Block.data);
        return () => {
            const cmac = new Cmac(cipher);
            cmac._subkey1 = k1;
            cmac._subkey2 = k2;
            return cmac;
        };
    }
    /**
     * Feed data into the CMAC computation.  Can be called multiple times
     * before {@link finish}.
     */
    async update(data) {
        const buf = this._buffer.data;
        let off = 0;
        // Fill partial buffer
        if (this._bufferPos > 0) {
            const need = Block.SIZE - this._bufferPos;
            if (data.length <= need) {
                buf.set(data, this._bufferPos);
                this._bufferPos += data.length;
                return;
            }
            buf.set(data.subarray(0, need), this._bufferPos);
            off = need;
            // Process full buffer
            const mac = await this.cipher.encryptBlock(buf);
            buf.set(mac);
            this._bufferPos = 0;
        }
        // Process complete blocks (keep at least 1 byte for finish padding logic)
        while (off + Block.SIZE < data.length) {
            xorInPlace(buf, data.subarray(off, off + Block.SIZE));
            const mac = await this.cipher.encryptBlock(buf);
            buf.set(mac);
            off += Block.SIZE;
        }
        // Store remainder
        const remaining = data.length - off;
        if (remaining > 0) {
            buf.set(data.subarray(off), 0);
            this._bufferPos = remaining;
        }
    }
    /**
     * Finalise and return the 16-byte CMAC tag.
     */
    async finish() {
        if (this._finished)
            throw new Error("CMAC already finished");
        this._finished = true;
        const buf = this._buffer.data;
        if (this._bufferPos === Block.SIZE) {
            // Complete block – XOR with K1
            xorInPlace(buf, this._subkey1);
        }
        else {
            // Incomplete block – pad with 10*0 and XOR with K2
            buf[this._bufferPos] ^= 0x80;
            xorInPlace(buf, this._subkey2);
        }
        return this.cipher.encryptBlock(buf);
    }
}
/* ====================================================================
 * AES-SIV  (RFC 5297)
 * ==================================================================== */
/** Error thrown on AES-SIV authentication failure. */
class AesSivError extends Error {
    constructor(message) {
        super(message);
        Object.setPrototypeOf(this, AesSivError.prototype);
    }
}
exports.AesSivError = AesSivError;
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
class AesSiv {
    cmacFactory;
    ctr;
    constructor(cmacFactory, ctr) {
        this.cmacFactory = cmacFactory;
        this.ctr = ctr;
    }
    /**
     * Derive the two AES-SIV sub-keys from an HKDF base key and salt.
     */
    static async importKey(hkdfKey, salt) {
        const cmacFactory = await Cmac.importKey(hkdfKey, salt);
        const ctr = await AesCtr.importKey(hkdfKey, salt);
        return new AesSiv(cmacFactory, ctr);
    }
    /**
     * Encrypt `plaintext` producing `[ SIV tag (16 bytes) ][ ciphertext ]`.
     */
    async seal(plaintext) {
        const total = Block.SIZE + plaintext.length;
        const output = new Uint8Array(total);
        const siv = await this.s2v(plaintext);
        output.set(siv);
        // Clear counter bits 31 and 63 for CTR mode
        clearSivBits(siv);
        const ct = await this.ctr.encryptCtr(siv, plaintext);
        output.set(ct, siv.length);
        return output;
    }
    /**
     * Decrypt and verify a ciphertext produced by {@link seal}.
     */
    async open(ciphertext) {
        if (ciphertext.length < Block.SIZE) {
            throw new AesSivError("AES-SIV: ciphertext is truncated");
        }
        const tag = ciphertext.subarray(0, Block.SIZE);
        const counter = new Uint8Array(Block.SIZE);
        counter.set(tag);
        clearSivBits(counter);
        const plaintext = await this.ctr.encryptCtr(counter, ciphertext.subarray(Block.SIZE));
        // Re-compute tag from the decrypted plaintext and compare
        const expectedTag = await this.s2v(plaintext);
        if (!constantTimeEqual(expectedTag, tag)) {
            // Zero out the plaintext to avoid leaking data on failure
            plaintext.fill(0);
            throw new AesSivError("AES-SIV: ciphertext verification failure!");
        }
        return plaintext;
    }
    /**
     * S2V (String-to-Vector) – the PRF used by AES-SIV.
     * Produces a 16-byte synthetic IV from the plaintext.
     */
    async s2v(plaintext) {
        const cmac = this.cmacFactory();
        const zero = new Block();
        const D = new Block();
        // D = CMAC(K, 0^128)
        await cmac.update(zero.data);
        D.data.set(await cmac.finish());
        // Process plaintext
        const cmac2 = this.cmacFactory();
        zero.clear();
        if (plaintext.length >= Block.SIZE) {
            // xorend: XOR D into the last 16 bytes of plaintext, then CMAC the lot.
            const xorStart = plaintext.length - Block.SIZE;
            zero.data.set(plaintext.subarray(xorStart));
            await cmac2.update(plaintext.subarray(0, xorStart));
        }
        else {
            // pad: plaintext || 10*0, then dbl(D) XOR padded
            zero.data.set(plaintext);
            zero.data[plaintext.length] = 0x80;
            D.dbl();
        }
        xorInPlace(zero.data, D.data);
        await cmac2.update(zero.data);
        return cmac2.finish();
    }
}
exports.AesSiv = AesSiv;
/* ====================================================================
 * Helpers
 * ==================================================================== */
/** XOR `b` into `a` in-place. */
function xorInPlace(a, b) {
    for (let i = 0; i < a.length; i++) {
        a[i] ^= b[i];
    }
}
/** Clear bits 31 and 63 (counting from the right) for SIV counter mode. */
function clearSivBits(siv) {
    siv[siv.length - 8] &= 0x7f;
    siv[siv.length - 4] &= 0x7f;
}
/** Constant-time comparison for two equal-length byte arrays. */
function constantTimeEqual(a, b) {
    if (a.length === 0 || b.length === 0)
        return false;
    if (a.length !== b.length)
        return false;
    let diff = 0;
    for (let i = 0; i < a.length; i++) {
        diff |= a[i] ^ b[i];
    }
    return ((1 & ((diff - 1) >>> 8)) !== 0);
}
/** Constant-time select: returns `a` when `cond` is 1, `b` when 0. */
function select(cond, a, b) {
    return (~(cond - 1) & a) | ((cond - 1) & b);
}
//# sourceMappingURL=aes-siv.js.map