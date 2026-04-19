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
export function stringToBuffer(str: string): ArrayBuffer {
  const encoded = new TextEncoder().encode(str);
  return encoded.buffer.slice(
    encoded.byteOffset,
    encoded.byteOffset + encoded.byteLength,
  );
}

/**
 * Decode an ArrayBuffer (or any buffer-like) into a UTF-8 string.
 */
export function bufferToString(buf: ArrayBuffer): string {
  return new TextDecoder().decode(new Uint8Array(buf));
}

/**
 * Convert a hex string (e.g. "deadbeef") to an ArrayBuffer.
 */
export function hexToBuffer(hex: string): ArrayBuffer {
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
export function bufferToHex(buf: ArrayBuffer): string {
  const view = new Uint8Array(buf);
  const parts: string[] = [];
  for (let i = 0; i < view.length; i++) {
    parts.push((view[i] >>> 4).toString(16));
    parts.push((view[i] & 0x0f).toString(16));
  }
  return parts.join("");
}

/**
 * Convert a base64 string to an ArrayBuffer.
 */
export function base64ToBuffer(b64: string): ArrayBuffer {
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
export function bufferToBase64(buf: ArrayBuffer): string {
  const arr = new Uint8Array(buf);
  const parts: string[] = [];
  for (let i = 0; i < arr.byteLength; i++) {
    parts.push(String.fromCharCode(arr[i]));
  }
  return btoa(parts.join(""));
}

/**
 * Return a view of the underlying ArrayBuffer for a Uint8Array,
 * trimmed to the exact byte range the view covers.
 */
export function toArrayBuffer(view: Uint8Array): ArrayBuffer {
  return view.buffer.slice(view.byteOffset, view.byteOffset + view.byteLength) as ArrayBuffer;
}

/**
 * Wrap an ArrayBuffer in a Node.js Buffer (zero-copy if possible).
 */
export function toNodeBuffer(buf: ArrayBuffer): Buffer {
  return Buffer.from(buf);
}
