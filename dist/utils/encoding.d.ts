/**
 * @module utils/encoding
 *
 * Low-level encoding helpers for converting between strings, ArrayBuffers,
 * hex and base64 representations.  These are used throughout the encryption
 * and networking layers.
 */
/**
 * Encode a UTF-8 string into an ArrayBuffer.
 */
export declare function stringToBuffer(str: string): ArrayBuffer;
/**
 * Decode an ArrayBuffer (or any buffer-like) into a UTF-8 string.
 */
export declare function bufferToString(buf: ArrayBuffer): string;
/**
 * Convert a hex string (e.g. "deadbeef") to an ArrayBuffer.
 */
export declare function hexToBuffer(hex: string): ArrayBuffer;
/**
 * Convert an ArrayBuffer to a lowercase hex string.
 */
export declare function bufferToHex(buf: ArrayBuffer): string;
/**
 * Convert a base64 string to an ArrayBuffer.
 */
export declare function base64ToBuffer(b64: string): ArrayBuffer;
/**
 * Convert an ArrayBuffer to a base64 string.
 */
export declare function bufferToBase64(buf: ArrayBuffer): string;
/**
 * Return a view of the underlying ArrayBuffer for a Uint8Array,
 * trimmed to the exact byte range the view covers.
 */
export declare function toArrayBuffer(view: Uint8Array): ArrayBuffer;
/**
 * Wrap an ArrayBuffer in a Node.js Buffer (zero-copy if possible).
 */
export declare function toNodeBuffer(buf: ArrayBuffer): Buffer;
//# sourceMappingURL=encoding.d.ts.map