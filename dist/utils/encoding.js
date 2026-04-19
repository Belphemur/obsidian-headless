"use strict";
/**
 * @module utils/encoding
 *
 * Low-level encoding helpers for converting between strings, ArrayBuffers,
 * hex and base64 representations.  These are used throughout the encryption
 * and networking layers.
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.stringToBuffer = stringToBuffer;
exports.bufferToString = bufferToString;
exports.hexToBuffer = hexToBuffer;
exports.bufferToHex = bufferToHex;
exports.base64ToBuffer = base64ToBuffer;
exports.bufferToBase64 = bufferToBase64;
exports.toArrayBuffer = toArrayBuffer;
exports.toNodeBuffer = toNodeBuffer;
/**
 * Encode a UTF-8 string into an ArrayBuffer.
 */
function stringToBuffer(str) {
    const encoded = new TextEncoder().encode(str);
    return encoded.buffer.slice(encoded.byteOffset, encoded.byteOffset + encoded.byteLength);
}
/**
 * Decode an ArrayBuffer (or any buffer-like) into a UTF-8 string.
 */
function bufferToString(buf) {
    return new TextDecoder().decode(new Uint8Array(buf));
}
/**
 * Convert a hex string (e.g. "deadbeef") to an ArrayBuffer.
 */
function hexToBuffer(hex) {
    const len = hex.length / 2;
    const buf = new ArrayBuffer(len);
    const view = new Uint8Array(buf);
    for (let i = 0; i < len; i++) {
        view[i] = parseInt(hex.substr(i * 2, 2), 16);
    }
    return buf;
}
/**
 * Convert an ArrayBuffer to a lowercase hex string.
 */
function bufferToHex(buf) {
    const view = new Uint8Array(buf);
    const parts = [];
    for (let i = 0; i < view.length; i++) {
        parts.push((view[i] >>> 4).toString(16));
        parts.push((view[i] & 0x0f).toString(16));
    }
    return parts.join("");
}
/**
 * Convert a base64 string to an ArrayBuffer.
 */
function base64ToBuffer(b64) {
    const raw = atob(b64);
    const len = raw.length;
    const arr = new Uint8Array(len);
    for (let i = 0; i < len; i++) {
        arr[i] = raw.charCodeAt(i);
    }
    return arr.buffer;
}
/**
 * Convert an ArrayBuffer to a base64 string.
 */
function bufferToBase64(buf) {
    const arr = new Uint8Array(buf);
    const parts = [];
    for (let i = 0; i < arr.byteLength; i++) {
        parts.push(String.fromCharCode(arr[i]));
    }
    return btoa(parts.join(""));
}
/**
 * Return a view of the underlying ArrayBuffer for a Uint8Array,
 * trimmed to the exact byte range the view covers.
 */
function toArrayBuffer(view) {
    return view.buffer.slice(view.byteOffset, view.byteOffset + view.byteLength);
}
/**
 * Wrap an ArrayBuffer in a Node.js Buffer (zero-copy if possible).
 */
function toNodeBuffer(buf) {
    return Buffer.from(buf);
}
//# sourceMappingURL=encoding.js.map