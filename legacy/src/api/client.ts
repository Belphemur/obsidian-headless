/**
 * HTTP API client for Obsidian services.
 *
 * Provides typed request functions for the Obsidian API, Publish API,
 * and per-vault publish hosts.
 */

const fetchFn = fetch;

// --- Base URLs ---

const OBSIDIAN_API_BASE = `https://${["api", "obsidian", "md"].join(".")}`;
const PUBLISH_API_BASE = "https://publish.obsidian.md";

// --- Interfaces ---

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

// --- Error Classes ---

/** Error thrown when the Obsidian API returns an error response. */
export class ApiError extends Error {
  /** The full parsed response body. */
  public readonly response: Record<string, unknown>;
  /** The error string from the response. */
  public readonly error: string;

  constructor(response: Record<string, unknown>, error: string) {
    super(error);
    this.name = "ApiError";
    this.response = response;
    this.error = error;
  }
}

/** Error thrown when a publish/host API returns an error response. */
export class RequestError extends Error {
  /** The error code from the response. */
  public readonly code: string;

  constructor(code: string, message: string) {
    super(message);
    this.name = "RequestError";
    this.code = code;
  }
}

// --- Helpers ---

/**
 * Determines the protocol for a given host.
 * Uses `http://` for localhost and 127.0.0.1, otherwise `https://`.
 */
function getProtocol(host: string): string {
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
export async function obsidianApiRequest(
  path: string,
  body: Record<string, unknown>,
  options?: { preflight?: boolean; headers?: Record<string, string> }
): Promise<Record<string, unknown>> {
  const url = `${OBSIDIAN_API_BASE}${path}`;

  if (options?.preflight) {
    await fetchFn(url, { method: "OPTIONS" });
  }

  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...options?.headers,
  };

  const response = await fetchFn(url, {
    method: "POST",
    headers,
    body: JSON.stringify(body),
  });

  const json = (await response.json()) as Record<string, unknown>;

  if (json.error) {
    throw new ApiError(json, json.error as string);
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
export async function publishApiRequest(
  path: string,
  body: Record<string, unknown>
): Promise<Record<string, unknown>> {
  const url = `${PUBLISH_API_BASE}${path}`;

  const response = await fetchFn(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });

  const json = (await response.json()) as Record<string, unknown>;

  if (json.code && json.message) {
    throw new RequestError(json.code as string, json.message as string);
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
export async function hostApiRequest(
  host: string,
  path: string,
  body: Record<string, unknown>
): Promise<Record<string, unknown>> {
  const protocol = getProtocol(host);
  const url = `${protocol}${host}${path}`;

  const response = await fetchFn(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });

  const json = (await response.json()) as Record<string, unknown>;

  if (json.code && json.message) {
    throw new RequestError(json.code as string, json.message as string);
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
export async function signIn(
  email: string,
  password: string,
  mfa?: string
): Promise<SignInResponse> {
  const body: Record<string, unknown> = { email, password };
  if (mfa) {
    body.mfa = mfa;
  }

  const response = await obsidianApiRequest("/user/signin", body, {
    preflight: true,
    headers: { Origin: "https://obsidian.md" },
  });

  return response as unknown as SignInResponse;
}

/**
 * Signs out of Obsidian, invalidating the given token.
 *
 * @param token - The authentication token to invalidate.
 */
export async function signOut(token: string): Promise<Record<string, unknown>> {
  return obsidianApiRequest("/user/signout", { token });
}

/**
 * Retrieves user account information.
 *
 * @param token - The authentication token.
 * @returns User info including uid, name, email, mfa status, license, and credit.
 */
export async function getUserInfo(token: string): Promise<UserInfoResponse> {
  const response = await obsidianApiRequest("/user/info", { token });
  return response as unknown as UserInfoResponse;
}

// --- Vault Endpoints ---

/**
 * Lists available vault regions.
 *
 * @param token - The authentication token.
 * @param host - Optional host override for region lookup.
 * @returns The regions response.
 */
export async function getRegions(
  token: string,
  host?: string
): Promise<Record<string, unknown>> {
  const body: Record<string, unknown> = { token };
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
export async function listVaults(
  token: string,
  encryptionVersion: number
): Promise<Record<string, unknown>> {
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
export async function createVault(
  token: string,
  name: string,
  keyhash: string,
  salt: string,
  region: string,
  encryptionVersion: number
): Promise<Record<string, unknown>> {
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
export async function validateAccess(
  token: string,
  vaultUid: string,
  keyhash: string,
  host: string,
  encryptionVersion: number
): Promise<Record<string, unknown>> {
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
export async function listPublishSites(
  token: string
): Promise<ListPublishSitesResponse> {
  const response = await obsidianApiRequest("/publish/list", { token });
  return response as unknown as ListPublishSitesResponse;
}

/**
 * Creates a new publish site.
 *
 * @param token - The authentication token.
 * @returns The created site information.
 */
export async function createPublishSite(
  token: string
): Promise<Record<string, unknown>> {
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
export async function setPublishSlug(
  token: string,
  id: string,
  host: string,
  slug: string
): Promise<Record<string, unknown>> {
  return publishApiRequest("/api/slug", { token, id, host, slug });
}

/**
 * Retrieves slug mappings for the given site IDs.
 *
 * @param token - The authentication token.
 * @param ids - Array of publish site IDs.
 * @returns The slug mappings response.
 */
export async function getPublishSlugs(
  token: string,
  ids: string[]
): Promise<Record<string, unknown>> {
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
export async function listPublishFiles(
  token: string,
  host: string,
  siteId: string
): Promise<Record<string, unknown>> {
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
export async function removePublishFile(
  token: string,
  host: string,
  siteId: string,
  path: string
): Promise<Record<string, unknown>> {
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
export async function uploadPublishFile(
  token: string,
  host: string,
  siteId: string,
  path: string,
  hash: string,
  data: Buffer | Uint8Array
): Promise<Record<string, unknown>> {
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

  const json = (await response.json()) as Record<string, unknown>;

  if (json.code && json.message) {
    throw new RequestError(json.code as string, json.message as string);
  }

  return json;
}
