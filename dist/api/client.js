"use strict";
/**
 * HTTP API client for Obsidian services.
 *
 * Provides typed request functions for the Obsidian API, Publish API,
 * and per-vault publish hosts.
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.RequestError = exports.ApiError = void 0;
exports.obsidianApiRequest = obsidianApiRequest;
exports.publishApiRequest = publishApiRequest;
exports.hostApiRequest = hostApiRequest;
exports.signIn = signIn;
exports.signOut = signOut;
exports.getUserInfo = getUserInfo;
exports.getRegions = getRegions;
exports.listVaults = listVaults;
exports.createVault = createVault;
exports.validateAccess = validateAccess;
exports.listPublishSites = listPublishSites;
exports.createPublishSite = createPublishSite;
exports.setPublishSlug = setPublishSlug;
exports.getPublishSlugs = getPublishSlugs;
exports.listPublishFiles = listPublishFiles;
exports.removePublishFile = removePublishFile;
exports.uploadPublishFile = uploadPublishFile;
const fetchFn = fetch;
// --- Base URLs ---
const OBSIDIAN_API_BASE = `https://${["api", "obsidian", "md"].join(".")}`;
const PUBLISH_API_BASE = "https://publish.obsidian.md";
// --- Error Classes ---
/** Error thrown when the Obsidian API returns an error response. */
class ApiError extends Error {
    /** The full parsed response body. */
    response;
    /** The error string from the response. */
    error;
    constructor(response, error) {
        super(error);
        this.name = "ApiError";
        this.response = response;
        this.error = error;
    }
}
exports.ApiError = ApiError;
/** Error thrown when a publish/host API returns an error response. */
class RequestError extends Error {
    /** The error code from the response. */
    code;
    constructor(code, message) {
        super(message);
        this.name = "RequestError";
        this.code = code;
    }
}
exports.RequestError = RequestError;
// --- Helpers ---
/**
 * Determines the protocol for a given host.
 * Uses `http://` for localhost and 127.0.0.1, otherwise `https://`.
 */
function getProtocol(host) {
    const hostname = host.split(":")[0];
    if (hostname === "localhost" || hostname === "127.0.0.1") {
        return "http://";
    }
    return "https://";
}
// --- Core Request Functions ---
/**
 * Sends a POST request to the Obsidian API.
 *
 * @param path - The API endpoint path (e.g. `/user/signin`).
 * @param body - The JSON request body.
 * @param options - Optional settings.
 * @param options.preflight - If true, sends an OPTIONS preflight request first.
 * @param options.headers - Additional headers to include in the request.
 * @returns The parsed JSON response.
 * @throws {ApiError} If the response contains an `error` field.
 */
async function obsidianApiRequest(path, body, options) {
    const url = `${OBSIDIAN_API_BASE}${path}`;
    if (options?.preflight) {
        await fetchFn(url, { method: "OPTIONS" });
    }
    const headers = {
        "Content-Type": "application/json",
        ...options?.headers,
    };
    const response = await fetchFn(url, {
        method: "POST",
        headers,
        body: JSON.stringify(body),
    });
    const json = (await response.json());
    if (json.error) {
        throw new ApiError(json, json.error);
    }
    return json;
}
/**
 * Sends a POST request to the Publish API.
 *
 * @param path - The API endpoint path (e.g. `/api/slug`).
 * @param body - The JSON request body.
 * @returns The parsed JSON response.
 * @throws {RequestError} If the response contains `code` and `message` fields.
 */
async function publishApiRequest(path, body) {
    const url = `${PUBLISH_API_BASE}${path}`;
    const response = await fetchFn(url, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
    });
    const json = (await response.json());
    if (json.code && json.message) {
        throw new RequestError(json.code, json.message);
    }
    return json;
}
/**
 * Sends a POST request to a custom publish host.
 *
 * Uses `http://` for localhost/127.0.0.1 hosts, otherwise `https://`.
 *
 * @param host - The target hostname (e.g. `publish-main.obsidian.md`).
 * @param path - The API endpoint path (e.g. `/api/list`).
 * @param body - The JSON request body.
 * @returns The parsed JSON response.
 * @throws {RequestError} If the response contains `code` and `message` fields.
 */
async function hostApiRequest(host, path, body) {
    const protocol = getProtocol(host);
    const url = `${protocol}${host}${path}`;
    const response = await fetchFn(url, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
    });
    const json = (await response.json());
    if (json.code && json.message) {
        throw new RequestError(json.code, json.message);
    }
    return json;
}
// --- Auth Endpoints ---
/**
 * Signs in to Obsidian with email and password.
 *
 * @param email - User email address.
 * @param password - User password.
 * @param mfa - Optional MFA/TOTP code.
 * @returns The sign-in response with token, name, and email.
 */
async function signIn(email, password, mfa) {
    const body = { email, password };
    if (mfa) {
        body.mfa = mfa;
    }
    const response = await obsidianApiRequest("/user/signin", body, {
        preflight: true,
        headers: { Origin: "https://obsidian.md" },
    });
    return response;
}
/**
 * Signs out of Obsidian, invalidating the given token.
 *
 * @param token - The authentication token to invalidate.
 */
async function signOut(token) {
    return obsidianApiRequest("/user/signout", { token });
}
/**
 * Retrieves user account information.
 *
 * @param token - The authentication token.
 * @returns User info including uid, name, email, mfa status, license, and credit.
 */
async function getUserInfo(token) {
    const response = await obsidianApiRequest("/user/info", { token });
    return response;
}
// --- Vault Endpoints ---
/**
 * Lists available vault regions.
 *
 * @param token - The authentication token.
 * @param host - Optional host override for region lookup.
 * @returns The regions response.
 */
async function getRegions(token, host) {
    const body = { token };
    if (host) {
        body.host = host;
    }
    return obsidianApiRequest("/vault/regions", body);
}
/**
 * Lists all vaults accessible to the user.
 *
 * @param token - The authentication token.
 * @param encryptionVersion - The supported encryption version number.
 * @returns The vault list response.
 */
async function listVaults(token, encryptionVersion) {
    return obsidianApiRequest("/vault/list", {
        token,
        supported_encryption_version: encryptionVersion,
    });
}
/**
 * Creates a new remote vault.
 *
 * @param token - The authentication token.
 * @param name - The vault name.
 * @param keyhash - The key hash for vault encryption.
 * @param salt - The encryption salt.
 * @param region - The region identifier for vault storage.
 * @param encryptionVersion - The encryption version to use.
 * @returns The create vault response.
 */
async function createVault(token, name, keyhash, salt, region, encryptionVersion) {
    return obsidianApiRequest("/vault/create", {
        token,
        name,
        keyhash,
        salt,
        region,
        encryption_version: encryptionVersion,
    });
}
/**
 * Validates access to a vault and retrieves connection details.
 *
 * @param token - The authentication token.
 * @param vaultUid - The unique identifier of the vault.
 * @param keyhash - The key hash for vault decryption.
 * @param host - The vault host.
 * @param encryptionVersion - The supported encryption version.
 * @returns The vault access response.
 */
async function validateAccess(token, vaultUid, keyhash, host, encryptionVersion) {
    return obsidianApiRequest("/vault/access", {
        token,
        uid: vaultUid,
        keyhash,
        host,
        supported_encryption_version: encryptionVersion,
    });
}
// --- Publish Endpoints ---
/**
 * Lists all publish sites for the user.
 *
 * @param token - The authentication token.
 * @returns The list of owned and shared publish sites.
 */
async function listPublishSites(token) {
    const response = await obsidianApiRequest("/publish/list", { token });
    return response;
}
/**
 * Creates a new publish site.
 *
 * @param token - The authentication token.
 * @returns The created site information.
 */
async function createPublishSite(token) {
    return obsidianApiRequest("/publish/create", { token });
}
/**
 * Sets the slug for a publish site.
 *
 * @param token - The authentication token.
 * @param id - The publish site ID.
 * @param host - The publish site host.
 * @param slug - The desired slug.
 * @returns The response from the publish API.
 */
async function setPublishSlug(token, id, host, slug) {
    return publishApiRequest("/api/slug", { token, id, host, slug });
}
/**
 * Retrieves slug mappings for the given site IDs.
 *
 * @param token - The authentication token.
 * @param ids - Array of publish site IDs.
 * @returns The slug mappings response.
 */
async function getPublishSlugs(token, ids) {
    return publishApiRequest("/api/slugs", { token, ids });
}
/**
 * Lists all published files for a site.
 *
 * @param token - The authentication token.
 * @param host - The publish host for the site.
 * @param siteId - The publish site ID.
 * @returns The list of published files.
 */
async function listPublishFiles(token, host, siteId) {
    return hostApiRequest(host, "/api/list", {
        token,
        id: siteId,
        version: 2,
    });
}
/**
 * Removes a published file from a site.
 *
 * @param token - The authentication token.
 * @param host - The publish host for the site.
 * @param siteId - The publish site ID.
 * @param path - The file path to remove.
 * @returns The removal response.
 */
async function removePublishFile(token, host, siteId, path) {
    return hostApiRequest(host, "/api/remove", {
        token,
        id: siteId,
        path,
    });
}
/**
 * Uploads a file to a publish site.
 *
 * Sends binary data with metadata in custom headers.
 *
 * @param token - The authentication token.
 * @param host - The publish host for the site.
 * @param siteId - The publish site ID.
 * @param path - The target file path on the site.
 * @param hash - The content hash of the file.
 * @param data - The binary file content.
 * @returns The upload response.
 * @throws {RequestError} If the response contains `code` and `message` fields.
 */
async function uploadPublishFile(token, host, siteId, path, hash, data) {
    const protocol = getProtocol(host);
    const url = `${protocol}${host}/api/upload`;
    const response = await fetchFn(url, {
        method: "POST",
        headers: {
            "Content-Type": "application/octet-stream",
            "obs-token": token,
            "obs-id": siteId,
            "obs-path": encodeURIComponent(path),
            "obs-hash": hash,
        },
        body: data,
    });
    const json = (await response.json());
    if (json.code && json.message) {
        throw new RequestError(json.code, json.message);
    }
    return json;
}
//# sourceMappingURL=client.js.map