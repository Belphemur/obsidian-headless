"use strict";
/**
 * @module utils/format
 *
 * Human-readable formatting utilities for file sizes and numbers.
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.formatBytes = formatBytes;
exports.formatNumber = formatNumber;
const SIZE_UNITS = ["B", "KB", "MB", "GB", "TB", "PB"];
/**
 * Format a byte count as a human-readable string (e.g. "1.23 MB").
 */
function formatBytes(bytes) {
    if (bytes <= 0)
        return "0 B";
    let unitIndex = SIZE_UNITS.length - 1;
    const base = 1024;
    for (let i = 0; i < SIZE_UNITS.length; i++) {
        if (bytes < Math.pow(base, i + 1)) {
            unitIndex = i;
            break;
        }
    }
    const value = bytes / Math.pow(base, unitIndex);
    return `${formatNumber(value, unitIndex === 0 ? 0 : 2)} ${SIZE_UNITS[unitIndex]}`;
}
/**
 * Format a number with thousands separators and fixed decimal places.
 */
function formatNumber(value, decimals = 2, decimalSep = ".", thousandsSep = ",") {
    const sign = value < 0 ? "-" : "";
    const abs = Math.abs(value);
    const intPart = parseInt(abs.toFixed(decimals), 10) + "";
    const mod = intPart.length > 3 ? intPart.length % 3 : 0;
    return (sign +
        (mod ? intPart.substr(0, mod) + thousandsSep : "") +
        intPart.substr(mod).replace(/(\d{3})(?=\d)/g, "$1" + thousandsSep) +
        (decimals
            ? decimalSep +
                Math.abs(abs - Math.floor(abs))
                    .toFixed(decimals)
                    .slice(2)
            : ""));
}
//# sourceMappingURL=format.js.map