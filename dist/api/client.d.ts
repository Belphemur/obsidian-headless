/**
 * HTTP API client for Obsidian services.
 *
 * Provides typed request functions for the Obsidian API, Publish API,
 * and per-vault publish hosts.
 */
/** Represents a remote vault. */
export interface Vault {
    id: string;
    uid: string;
    name: string;
    password: string;
    salt: string;
    created: number;
    host: string;
    size: number;
    encryption_version?: number;
}
/** Represents a publish site. */
export interface PublishSite {
    id: string;
    slug: string;
    host: string;
    created: number;
}
/** Sign-in response payload. */
export interface SignInResponse {
    token: string;
    name: string;
    email: string;
}
/** User info response payload. */
export interface UserInfoResponse {
    uid: string;
    name: string;
    email: string;
    mfa: boolean;
    license: string;
    credit: number;
    discount: number;
}
/** Vault region descriptor. */
export interface Region {
    id: string;
    name: string;
}
/** Published file entry. */
export interface PublishFile {
    path: string;
    hash: string;
    size: number;
}
/** List publish sites response. */
export interface ListPublishSitesResponse {
    sites: PublishSite[];
    shared: PublishSite[];
}
/** Slug mapping entry. */
export interface SlugMapping {
    id: string;
    slug: string;
}
/** Error thrown when the Obsidian API returns an error response. */
export declare class ApiError extends Error {
    /** The full parsed response body. */
    readonly response: Record<string, unknown>;
    /** The error string from the response. */
    readonly error: string;
    constructor(response: Record<string, unknown>, error: string);
}
/** Error thrown when a publish/host API returns an error response. */
export declare class RequestError extends Error {
    /** The error code from the response. */
    readonly code: string;
    constructor(code: string, message: string);
}
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
export declare function obsidianApiRequest(path: string, body: Record<string, unknown>, options?: {
    preflight?: boolean;
    headers?: Record<string, string>;
}): Promise<Record<string, unknown>>;
/**
 * Sends a POST request to the Publish API.
 *
 * @param path - The API endpoint path (e.g. `/api/slug`).
 * @param body - The JSON request body.
 * @returns The parsed JSON response.
 * @throws {RequestError} If the response contains `code` and `message` fields.
 */
export declare function publishApiRequest(path: string, body: Record<string, unknown>): Promise<Record<string, unknown>>;
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
export declare function hostApiRequest(host: string, path: string, body: Record<string, unknown>): Promise<Record<string, unknown>>;
/**
 * Signs in to Obsidian with email and password.
 *
 * @param email - User email address.
 * @param password - User password.
 * @param mfa - Optional MFA/TOTP code.
 * @returns The sign-in response with token, name, and email.
 */
export declare function signIn(email: string, password: string, mfa?: string): Promise<SignInResponse>;
/**
 * Signs out of Obsidian, invalidating the given token.
 *
 * @param token - The authentication token to invalidate.
 */
export declare function signOut(token: string): Promise<Record<string, unknown>>;
/**
 * Retrieves user account information.
 *
 * @param token - The authentication token.
 * @returns User info including uid, name, email, mfa status, license, and credit.
 */
export declare function getUserInfo(token: string): Promise<UserInfoResponse>;
/**
 * Lists available vault regions.
 *
 * @param token - The authentication token.
 * @param host - Optional host override for region lookup.
 * @returns The regions response.
 */
export declare function getRegions(token: string, host?: string): Promise<Record<string, unknown>>;
/**
 * Lists all vaults accessible to the user.
 *
 * @param token - The authentication token.
 * @param encryptionVersion - The supported encryption version number.
 * @returns The vault list response.
 */
export declare function listVaults(token: string, encryptionVersion: number): Promise<Record<string, unknown>>;
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
export declare function createVault(token: string, name: string, keyhash: string, salt: string, region: string, encryptionVersion: number): Promise<Record<string, unknown>>;
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
export declare function validateAccess(token: string, vaultUid: string, keyhash: string, host: string, encryptionVersion: number): Promise<Record<string, unknown>>;
/**
 * Lists all publish sites for the user.
 *
 * @param token - The authentication token.
 * @returns The list of owned and shared publish sites.
 */
export declare function listPublishSites(token: string): Promise<ListPublishSitesResponse>;
/**
 * Creates a new publish site.
 *
 * @param token - The authentication token.
 * @returns The created site information.
 */
export declare function createPublishSite(token: string): Promise<Record<string, unknown>>;
/**
 * Sets the slug for a publish site.
 *
 * @param token - The authentication token.
 * @param id - The publish site ID.
 * @param host - The publish site host.
 * @param slug - The desired slug.
 * @returns The response from the publish API.
 */
export declare function setPublishSlug(token: string, id: string, host: string, slug: string): Promise<Record<string, unknown>>;
/**
 * Retrieves slug mappings for the given site IDs.
 *
 * @param token - The authentication token.
 * @param ids - Array of publish site IDs.
 * @returns The slug mappings response.
 */
export declare function getPublishSlugs(token: string, ids: string[]): Promise<Record<string, unknown>>;
/**
 * Lists all published files for a site.
 *
 * @param token - The authentication token.
 * @param host - The publish host for the site.
 * @param siteId - The publish site ID.
 * @returns The list of published files.
 */
export declare function listPublishFiles(token: string, host: string, siteId: string): Promise<Record<string, unknown>>;
/**
 * Removes a published file from a site.
 *
 * @param token - The authentication token.
 * @param host - The publish host for the site.
 * @param siteId - The publish site ID.
 * @param path - The file path to remove.
 * @returns The removal response.
 */
export declare function removePublishFile(token: string, host: string, siteId: string, path: string): Promise<Record<string, unknown>>;
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
export declare function uploadPublishFile(token: string, host: string, siteId: string, path: string, hash: string, data: Buffer | Uint8Array): Promise<Record<string, unknown>>;
//# sourceMappingURL=client.d.ts.map