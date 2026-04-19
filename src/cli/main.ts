#!/usr/bin/env node
/**
 * @module cli/main
 * Main CLI entry point for the Obsidian headless client.
 * Uses Commander.js to define all user-facing commands for vault sync and publish operations.
 */

import { Command } from "commander";
import fs from "node:fs";
import path from "node:path";
import crypto from "node:crypto";

import {
  signIn,
  signOut,
  getUserInfo,
  getRegions,
  listVaults,
  createVault,
  validateAccess,
  listPublishSites,
  createPublishSite,
  setPublishSlug,
  getPublishSlugs,
  ApiError,
} from "../api/client.js";
import {
  getAuthToken,
  saveAuthToken,
  clearAuthToken,
  readSyncConfig,
  writeSyncConfig,
  ensureSyncDir,
  listLocalVaults,
  findSyncConfigByPath,
  getDefaultDeviceName,
  getLogPath,
  getLockPath,
  removeSyncConfig,
  readPublishConfig,
  writePublishConfig,
  findPublishConfigByPath,
  removePublishConfig,
  parseFileTypes,
  parseConfigCategories,
  validateConfigDir,
  setupLogFile,
  type SyncConfig,
  type PublishConfig,
} from "../config/index.js";
import {
  deriveKey,
  computeKeyHash,
  createEncryptionProvider,
} from "../encryption/index.js";
import {
  bufferToHex,
  base64ToBuffer,
  bufferToBase64,
} from "../utils/encoding.js";
import { SyncEngine } from "../sync/engine.js";
import { FileLock, LockError } from "../sync/lock.js";
import { PublishEngine } from "../publish/engine.js";

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const PROGRAM_NAME = "ob";
const MAX_ENCRYPTION_VERSION = 3;
const SUPPORTED_ENCRYPTION_VERSION = 3;

// ---------------------------------------------------------------------------
// Version from package.json
// ---------------------------------------------------------------------------

const pkgPath = path.join(path.dirname(process.argv[1] || "."), "..", "package.json");
const { version } = JSON.parse(fs.readFileSync(pkgPath, "utf-8"));

// ---------------------------------------------------------------------------
// Program setup
// ---------------------------------------------------------------------------

const program = new Command();
program
  .name(PROGRAM_NAME)
  .description("Headless client for Obsidian services")
  .version(version);

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/**
 * Ensures the user is authenticated, returning the stored auth token.
 * Exits with code 2 if no token is found.
 */
function requireAuth(): string {
  const token = getAuthToken();
  if (!token) {
    console.error(
      `No account logged in. Run "${PROGRAM_NAME} login" first.`,
    );
    process.exit(2);
  }
  return token;
}

/**
 * Prompts the user for interactive text input.
 *
 * When stdin is a TTY, uses raw mode to read character-by-character.
 * For passwords (showInput=false), echoes asterisks.
 * When stdin is not a TTY, reads all of stdin.
 *
 * @param message - The prompt message to display
 * @param showInput - Whether to display input characters (false for passwords)
 * @returns The user-entered string
 */
function promptInput(message: string, showInput = false): Promise<string> {
  return new Promise((resolve, reject) => {
    process.stdout.write(message);

    if (!process.stdin.isTTY) {
      // Non-interactive: read all stdin
      let data = "";
      process.stdin.setEncoding("utf-8");
      process.stdin.on("data", (chunk) => {
        data += chunk;
      });
      process.stdin.on("end", () => {
        resolve(data.trim());
      });
      process.stdin.resume();
      return;
    }

    // Interactive TTY mode
    const input: string[] = [];
    process.stdin.setRawMode(true);
    process.stdin.resume();
    process.stdin.setEncoding("utf-8");

    const onData = (char: string) => {
      const code = char.charCodeAt(0);

      // Ctrl+C
      if (char === "\u0003") {
        process.stdin.setRawMode(false);
        process.stdin.pause();
        process.stdin.removeListener("data", onData);
        process.stdout.write("\n");
        process.exit(130);
      }

      // Enter
      if (char === "\r" || char === "\n") {
        process.stdin.setRawMode(false);
        process.stdin.pause();
        process.stdin.removeListener("data", onData);
        process.stdout.write("\n");
        resolve(input.join(""));
        return;
      }

      // Backspace / Delete
      if (char === "\u007f" || char === "\b") {
        if (input.length > 0) {
          input.pop();
          process.stdout.write("\b \b");
        }
        return;
      }

      // Regular character
      input.push(char);
      if (showInput) {
        process.stdout.write(char);
      } else {
        process.stdout.write("*");
      }
    };

    process.stdin.on("data", onData);
  });
}

/**
 * Checks if the native btime module is available on platforms that support it
 * (Windows and macOS). Prints a warning if the module cannot be loaded since
 * birth-time tracking improves sync accuracy.
 */
function checkBtimeModule(): void {
  if (process.platform !== "win32" && process.platform !== "darwin") {
    return;
  }

  try {
    // Attempt to load the native btime module
    const btimePath = path.join(__dirname, "..", "..", "btime");
    require(btimePath);
  } catch {
    console.warn(
      "Warning: Native btime module not loaded. File birth-time tracking may be less accurate.",
    );
  }
}

/**
 * Prints a formatted display of a sync vault configuration.
 *
 * @param config - The sync configuration to display
 */
function printSyncConfig(config: SyncConfig): void {
  const modeLabels: Record<string, string> = {
    bidirectional: "bidirectional",
    "pull-only": "pull-only",
    "mirror-remote": "mirror-remote",
  };

  const mode = modeLabels[config.syncMode ?? "bidirectional"] ?? config.syncMode ?? "bidirectional";
  const conflict = config.conflictStrategy ?? "merge";
  const device = config.deviceName ?? getDefaultDeviceName();
  const configDir = config.configDir ?? ".obsidian";

  console.log(`  Vault: ${config.vaultName} (${config.vaultId})`);
  console.log(`  Location: ${config.vaultPath}`);
  console.log(`  Sync mode: ${mode}`);
  console.log(`  Conflict strategy: ${conflict}`);
  console.log(`  Device name: ${device}`);
  console.log(`  Config directory: ${configDir}`);

  if (config.allowTypes && config.allowTypes.length > 0) {
    console.log(`  File types: ${config.allowTypes.join(", ")}`);
  } else {
    console.log(`  File types: image, audio, pdf, video`);
  }

  if (config.allowSpecialFiles && config.allowSpecialFiles.length > 0) {
    console.log(`  Configs: ${config.allowSpecialFiles.join(", ")}`);
  } else {
    console.log(`  Configs: none (config syncing disabled)`);
  }

  if (config.ignoreFolders && config.ignoreFolders.length > 0) {
    console.log(`  Excluded folders: ${config.ignoreFolders.join(", ")}`);
  }
}

/**
 * Prints a formatted display of a publish site configuration.
 *
 * @param config - The publish configuration to display
 */
function printPublishConfig(config: PublishConfig): void {
  console.log(`  Site ID: ${config.siteId}`);
  console.log(`  Host: ${config.host}`);
  console.log(`  Location: ${config.vaultPath}`);
  if (config.includes && config.includes.length > 0) {
    console.log(`  Includes: ${config.includes.join(", ")}`);
  }
  if (config.excludes && config.excludes.length > 0) {
    console.log(`  Excludes: ${config.excludes.join(", ")}`);
  }
}

// ---------------------------------------------------------------------------
// Commands
// ---------------------------------------------------------------------------

/**
 * **login** — Authenticate with the Obsidian account.
 *
 * If already logged in and no arguments provided, displays current user info.
 * Otherwise prompts for email, password, and optional 2FA code.
 * Supports non-interactive use via --email, --password, and --mfa flags.
 */
program
  .command("login")
  .description("Log in to your Obsidian account")
  .option("--email <email>", "Account email address")
  .option("--password <password>", "Account password")
  .option("--mfa <code>", "Two-factor authentication code")
  .action(async (opts) => {
    const existingToken = getAuthToken();

    // If already logged in and no args, show current user info
    if (existingToken && !opts.email && !opts.password) {
      try {
        const user = await getUserInfo(existingToken);
        console.log(`Logged in as: ${user.email}`);
        if (user.name) {
          console.log(`Name: ${user.name}`);
        }
        return;
      } catch {
        // Token may be invalid, proceed with login
      }
    }

    // If already logged in, sign out old session
    if (existingToken) {
      try {
        await signOut(existingToken);
      } catch {
        // Ignore sign-out errors
      }
      clearAuthToken();
    }

    const email = opts.email ?? (await promptInput("Email: ", true));
    const password = opts.password ?? (await promptInput("Password: "));

    let mfa = opts.mfa;

    try {
      const result = await signIn(email, password, mfa);
      saveAuthToken(result.token);
      console.log("Login successful.");
    } catch (err) {
      if (err instanceof ApiError && err.message.includes("2FA")) {
        // Need 2FA code
        if (!mfa) {
          mfa = await promptInput("2FA code: ", true);
        }
        try {
          const result = await signIn(email, password, mfa);
          saveAuthToken(result.token);
          console.log("Login successful.");
        } catch (err2) {
          console.error(
            `Login failed: ${err2 instanceof Error ? err2.message : String(err2)}`,
          );
          process.exit(1);
        }
      } else {
        console.error(
          `Login failed: ${err instanceof Error ? err.message : String(err)}`,
        );
        process.exit(1);
      }
    }
  });

/**
 * **logout** — Sign out and clear the stored authentication token.
 */
program
  .command("logout")
  .description("Log out of your Obsidian account")
  .action(async () => {
    const token = getAuthToken();
    if (token) {
      try {
        await signOut(token);
      } catch {
        // Ignore errors during sign-out
      }
    }
    clearAuthToken();
    console.log("Logged out.");
  });

/**
 * **sync-list-remote** — List all remote vaults associated with the account.
 *
 * Displays vault ID, name, and region for each remote vault.
 */
program
  .command("sync-list-remote")
  .description("List remote vaults")
  .action(async () => {
    const token = requireAuth();
    const vaults = await listVaults(token, SUPPORTED_ENCRYPTION_VERSION);

    if (vaults.length === 0) {
      console.log("No remote vaults found.");
      return;
    }

    console.log("Remote vaults:");
    for (const vault of vaults) {
      console.log(`  ${vault.id}  ${vault.name}  (${vault.region ?? "default"})`);
    }
  });

/**
 * **sync-list-local** — List locally configured sync vaults.
 *
 * Shows the local path and remote host for each vault that has been set up
 * for syncing on this machine.
 */
program
  .command("sync-list-local")
  .description("List locally configured vaults")
  .action(async () => {
    const configs = listLocalVaults();

    if (configs.length === 0) {
      console.log("No locally configured vaults.");
      return;
    }

    console.log("Local vaults:");
    for (const config of configs) {
      console.log(`  ${config.path}  (${config.host ?? "default"})`);
    }
  });

/**
 * **sync-create-remote** — Create a new remote vault.
 *
 * Creates a vault with the given name and optional encryption settings.
 * For end-to-end encryption (e2ee), prompts for a password to derive the
 * encryption key. Validates the chosen region against available regions.
 *
 * @example
 * ob sync-create-remote --name "My Vault" --encryption e2ee
 * ob sync-create-remote --name "Work" --encryption standard --region us-east
 */
program
  .command("sync-create-remote")
  .description("Create a new remote vault")
  .requiredOption("--name <name>", "Vault name")
  .option("--encryption <type>", "Encryption type: standard or e2ee", "e2ee")
  .option("--password <password>", "Encryption password (for e2ee)")
  .option("--region <region>", "Server region")
  .action(async (opts) => {
    const token = requireAuth();

    let password = opts.password;
    if (opts.encryption !== "standard" && !password) {
      password = await promptInput("Encryption password: ");
      const confirm = await promptInput("Confirm password: ");
      if (password !== confirm) {
        console.error("Passwords do not match.");
        process.exit(1);
      }
    }

    // Validate region
    if (opts.region) {
      const regions = await getRegions(token);
      const valid = regions.some(
        (r: { id?: string; name?: string }) =>
          r.id === opts.region || r.name === opts.region,
      );
      if (!valid) {
        console.error(
          `Invalid region "${opts.region}". Available: ${regions.map((r: { id?: string; name?: string }) => r.id ?? r.name).join(", ")}`,
        );
        process.exit(1);
      }
    }

    let keyHash: string | undefined;
    let salt: string | undefined;

    if (opts.encryption !== "standard" && password) {
      // Generate random 16-byte salt
      salt = bufferToHex(crypto.randomBytes(16));
      const key = await deriveKey(password, salt);
      keyHash = await computeKeyHash(key);
    }

    const encVersion =
      opts.encryption === "standard" ? 0 : SUPPORTED_ENCRYPTION_VERSION;

    const vault = await createVault(token, {
      name: opts.name,
      encryptionVersion: encVersion,
      keyHash,
      salt,
      region: opts.region,
    });

    console.log(`Vault created successfully.`);
    console.log(`  ID: ${vault.id}`);
    console.log(`  Name: ${vault.name}`);
    if (vault.region) {
      console.log(`  Region: ${vault.region}`);
    }
  });

/**
 * **sync-setup** — Set up a local directory for vault syncing.
 *
 * Associates a local path with a remote vault. If the vault uses end-to-end
 * encryption, prompts for the password and validates it against the remote.
 * Stores the derived encryption key locally for subsequent sync operations.
 *
 * @example
 * ob sync-setup --vault "My Vault" --path ~/notes
 * ob sync-setup --vault abc123 --device-name "server-1"
 */
program
  .command("sync-setup")
  .description("Set up sync for a vault")
  .requiredOption("--vault <idOrName>", "Vault ID or name")
  .option("--path <path>", "Local vault directory", ".")
  .option("--password <password>", "Encryption password")
  .option("--device-name <name>", "Device name for this client")
  .option("--config-dir <dir>", "Config directory name", ".obsidian")
  .action(async (opts) => {
    const token = requireAuth();
    const vaults = await listVaults(token, SUPPORTED_ENCRYPTION_VERSION);

    // Find vault by ID or name
    let vault = vaults.find((v) => v.id === opts.vault);
    if (!vault) {
      const byName = vaults.filter((v) => v.name === opts.vault);
      if (byName.length > 1) {
        console.error(
          `Ambiguous vault name "${opts.vault}". Multiple vaults found. Use the vault ID instead.`,
        );
        for (const v of byName) {
          console.error(`  ${v.id}  ${v.name}`);
        }
        process.exit(1);
      }
      vault = byName[0];
    }

    if (!vault) {
      console.error(`Vault "${opts.vault}" not found.`);
      process.exit(1);
    }

    const localPath = path.resolve(opts.path);
    let encKey: string | undefined;
    let keySalt: string | undefined;

    // Handle encryption
    if (vault.encryptionVersion && vault.encryptionVersion > 0) {
      const password =
        opts.password ?? (await promptInput("Encryption password: "));

      if (!vault.salt) {
        console.error("Vault is encrypted but has no salt. Cannot proceed.");
        process.exit(1);
      }

      keySalt = vault.salt;
      const key = await deriveKey(password, vault.salt);
      const keyHash = await computeKeyHash(key);

      // Validate access using the derived key
      const valid = await validateAccess(token, vault.id, keyHash);
      if (!valid) {
        console.error("Invalid encryption password.");
        process.exit(1);
      }

      encKey = bufferToBase64(key);
    }

    const deviceName = opts.deviceName ?? getDefaultDeviceName();
    ensureSyncDir(vault.id);

    const config: SyncConfig = {
      vaultId: vault.id,
      name: vault.name,
      path: localPath,
      host: vault.host ?? undefined,
      encryptionVersion: vault.encryptionVersion ?? 0,
      key: encKey,
      salt: keySalt,
      deviceName,
      configDir: opts.configDir,
    };

    writeSyncConfig(vault.id, config);

    console.log(`Sync configured for vault "${vault.name}".`);
    console.log(`  Path: ${localPath}`);

    // Warn if the local directory already has files
    if (fs.existsSync(localPath)) {
      const entries = fs.readdirSync(localPath);
      if (entries.length > 0) {
        console.warn(
          "\nWarning: The local directory already contains files. Existing files will be merged during sync.",
        );
      }
    }
  });

/**
 * **sync-config** — Modify sync configuration for a vault.
 *
 * Updates settings like conflict strategy, excluded folders, file types,
 * device name, sync mode, and config categories for an already-configured vault.
 *
 * @example
 * ob sync-config --path ~/notes --mode pull-only --conflict-strategy merge
 * ob sync-config --excluded-folders "archive,temp"
 */
program
  .command("sync-config")
  .description("Update sync configuration")
  .option("--path <path>", "Vault path", ".")
  .option("--conflict-strategy <strategy>", "Conflict strategy: merge or conflict")
  .option("--excluded-folders <folders>", "Comma-separated folders to exclude")
  .option("--file-types <types>", "Comma-separated file types to sync")
  .option("--configs <categories>", "Comma-separated config categories")
  .option("--device-name <name>", "Device name")
  .option("--mode <mode>", "Sync mode: bidirectional, pull-only, or mirror-remote")
  .option("--config-dir <dir>", "Config directory name")
  .action(async (opts) => {
    const localPath = path.resolve(opts.path);
    const config = findSyncConfigByPath(localPath);

    if (!config) {
      console.error(
        `No sync configuration found for path "${localPath}". Run "${PROGRAM_NAME} sync-setup" first.`,
      );
      process.exit(1);
    }

    let changed = false;

    if (opts.conflictStrategy !== undefined) {
      config.conflictStrategy = opts.conflictStrategy;
      changed = true;
    }
    if (opts.excludedFolders !== undefined) {
      config.excludedFolders = opts.excludedFolders
        .split(",")
        .map((f: string) => f.trim())
        .filter(Boolean);
      changed = true;
    }
    if (opts.fileTypes !== undefined) {
      config.fileTypes = parseFileTypes(opts.fileTypes);
      changed = true;
    }
    if (opts.configs !== undefined) {
      config.configs = parseConfigCategories(opts.configs);
      changed = true;
    }
    if (opts.deviceName !== undefined) {
      config.deviceName = opts.deviceName;
      changed = true;
    }
    if (opts.mode !== undefined) {
      config.mode = opts.mode;
      changed = true;
    }
    if (opts.configDir !== undefined) {
      validateConfigDir(opts.configDir);
      config.configDir = opts.configDir;
      changed = true;
    }

    if (!changed) {
      console.log("No changes specified. Current configuration:");
      printSyncConfig(config);
      return;
    }

    writeSyncConfig(config.vaultId, config);
    console.log("Sync configuration updated.");
    printSyncConfig(config);
  });

/**
 * **sync-status** — Display the current sync configuration and status.
 *
 * Shows all configured settings for the vault at the given path.
 */
program
  .command("sync-status")
  .description("Show sync status for a vault")
  .option("--path <path>", "Vault path", ".")
  .action(async (opts) => {
    const localPath = path.resolve(opts.path);
    const config = findSyncConfigByPath(localPath);

    if (!config) {
      console.error(
        `No sync configuration found for path "${localPath}".`,
      );
      process.exit(1);
    }

    console.log("Sync configuration:");
    printSyncConfig(config);
  });

/**
 * **sync-unlink** — Disconnect a local vault from remote sync.
 *
 * Removes the sync configuration and state directory for the vault,
 * but does not delete the local files or the remote vault.
 */
program
  .command("sync-unlink")
  .description("Disconnect a vault from sync")
  .option("--path <path>", "Vault path", ".")
  .action(async (opts) => {
    const localPath = path.resolve(opts.path);
    const config = findSyncConfigByPath(localPath);

    if (!config) {
      console.error(
        `No sync configuration found for path "${localPath}".`,
      );
      process.exit(1);
    }

    removeSyncConfig(config.vaultId);
    console.log(`Vault "${config.name}" unlinked from sync.`);
  });

/**
 * **sync** — Run the vault synchronization process.
 *
 * Performs a full sync cycle between the local vault and the remote.
 * In continuous mode, watches for changes and syncs automatically.
 * Acquires a file lock to prevent concurrent sync instances.
 *
 * Handles graceful shutdown on SIGINT/SIGTERM.
 *
 * @example
 * ob sync --path ~/notes
 * ob sync --path ~/notes --continuous
 */
program
  .command("sync")
  .description("Sync a vault")
  .option("--path <path>", "Vault path", ".")
  .option("--continuous", "Run continuously, watching for changes")
  .action(async (opts) => {
    const token = requireAuth();
    const localPath = path.resolve(opts.path);
    const config = findSyncConfigByPath(localPath);

    if (!config) {
      console.error(
        `No sync configuration found for path "${localPath}". Run "${PROGRAM_NAME} sync-setup" first.`,
      );
      process.exit(1);
    }

    // Setup encryption provider
    let encryptionProvider;
    if (config.encryptionVersion && config.encryptionVersion > 0 && config.key) {
      const keyBuffer = base64ToBuffer(config.key);
      encryptionProvider = createEncryptionProvider(
        config.encryptionVersion,
        keyBuffer,
      );
    }

    // Setup log file
    const logPath = getLogPath(config.vaultId);
    setupLogFile(logPath);

    // Check btime module availability
    checkBtimeModule();

    // Acquire file lock
    const lockPath = getLockPath(config.vaultId);
    const lock = new FileLock(lockPath);

    try {
      lock.acquire();
    } catch (err) {
      if (err instanceof LockError) {
        console.error(
          "Another sync instance is already running for this vault.",
        );
        process.exit(1);
      }
      throw err;
    }

    // Create sync engine
    const engine = new SyncEngine({
      token,
      config,
      encryptionProvider,
      continuous: opts.continuous ?? false,
    });

    // Handle graceful shutdown
    let stopping = false;
    const shutdown = async () => {
      if (stopping) return;
      stopping = true;
      console.log("\nStopping sync...");
      await engine.stop();
    };

    process.on("SIGINT", shutdown);
    process.on("SIGTERM", shutdown);

    try {
      console.log("Sync configuration:");
      printSyncConfig(config);
      console.log("");

      await engine.run();
    } finally {
      lock.release();
      process.removeListener("SIGINT", shutdown);
      process.removeListener("SIGTERM", shutdown);
    }
  });

/**
 * **publish-list-sites** — List all Obsidian Publish sites on the account.
 *
 * Displays each site's ID and associated slug (public URL path).
 */
program
  .command("publish-list-sites")
  .description("List publish sites")
  .action(async () => {
    const token = requireAuth();
    const sites = await listPublishSites(token);

    if (sites.length === 0) {
      console.log("No publish sites found.");
      return;
    }

    const slugs = await getPublishSlugs(token);

    console.log("Publish sites:");
    for (const site of sites) {
      const slug = slugs.find((s) => s.siteId === site.id);
      const slugDisplay = slug ? ` (${slug.slug})` : "";
      console.log(`  ${site.id}  ${site.name ?? "Untitled"}${slugDisplay}`);
    }
  });

/**
 * **publish-create-site** — Create a new Obsidian Publish site.
 *
 * Creates a site and assigns the specified slug (public URL path).
 *
 * @example
 * ob publish-create-site --slug my-digital-garden
 */
program
  .command("publish-create-site")
  .description("Create a new publish site")
  .requiredOption("--slug <slug>", "Site slug (URL path)")
  .action(async (opts) => {
    const token = requireAuth();
    const site = await createPublishSite(token);
    await setPublishSlug(token, site.id, opts.slug);
    console.log(`Publish site created.`);
    console.log(`  ID: ${site.id}`);
    console.log(`  Slug: ${opts.slug}`);
  });

/**
 * **publish-setup** — Connect a local vault to a publish site.
 *
 * Associates a local directory with an existing publish site for deploying
 * content. The site can be identified by its ID or slug.
 *
 * @example
 * ob publish-setup --site my-digital-garden --path ~/notes
 */
program
  .command("publish-setup")
  .description("Set up publish for a vault")
  .requiredOption("--site <idOrSlug>", "Site ID or slug")
  .option("--path <path>", "Local vault directory", ".")
  .action(async (opts) => {
    const token = requireAuth();
    const sites = await listPublishSites(token);
    const slugs = await getPublishSlugs(token);

    // Find site by ID first
    let site = sites.find((s) => s.id === opts.site);

    // Fallback to slug lookup
    if (!site) {
      const slugEntry = slugs.find((s) => s.slug === opts.site);
      if (slugEntry) {
        site = sites.find((s) => s.id === slugEntry.siteId);
      }
    }

    if (!site) {
      console.error(`Publish site "${opts.site}" not found.`);
      process.exit(1);
    }

    const localPath = path.resolve(opts.path);
    const siteSlug = slugs.find((s) => s.siteId === site!.id);

    const config: PublishConfig = {
      siteId: site.id,
      slug: siteSlug?.slug,
      path: localPath,
    };

    writePublishConfig(site.id, config);
    console.log(`Publish configured for site "${siteSlug?.slug ?? site.id}".`);
    console.log(`  Path: ${localPath}`);
  });

/**
 * **publish** — Publish changes from the local vault to the site.
 *
 * Scans for new, changed, and deleted files then uploads/removes them.
 * Supports dry-run mode to preview changes without publishing, and
 * --yes to skip the confirmation prompt.
 *
 * @example
 * ob publish --path ~/notes
 * ob publish --path ~/notes --dry-run
 * ob publish --path ~/notes --yes --all
 */
program
  .command("publish")
  .description("Publish changes to site")
  .option("--path <path>", "Vault path", ".")
  .option("--dry-run", "Show changes without publishing")
  .option("--yes", "Skip confirmation prompt")
  .option("--all", "Publish all files, not just changed ones")
  .action(async (opts) => {
    const token = requireAuth();
    const localPath = path.resolve(opts.path);
    const config = findPublishConfigByPath(localPath);

    if (!config) {
      console.error(
        `No publish configuration found for path "${localPath}". Run "${PROGRAM_NAME} publish-setup" first.`,
      );
      process.exit(1);
    }

    const engine = new PublishEngine({
      token,
      config,
      all: opts.all ?? false,
    });

    const changes = await engine.scan();

    const newFiles = changes.filter((c) => c.type === "new");
    const modified = changes.filter((c) => c.type === "changed");
    const deleted = changes.filter((c) => c.type === "deleted");

    if (changes.length === 0) {
      console.log("No changes to publish.");
      return;
    }

    console.log("Changes to publish:");
    if (newFiles.length > 0) {
      console.log(`  New: ${newFiles.length} file(s)`);
      for (const f of newFiles) {
        console.log(`    + ${f.path}`);
      }
    }
    if (modified.length > 0) {
      console.log(`  Changed: ${modified.length} file(s)`);
      for (const f of modified) {
        console.log(`    ~ ${f.path}`);
      }
    }
    if (deleted.length > 0) {
      console.log(`  Deleted: ${deleted.length} file(s)`);
      for (const f of deleted) {
        console.log(`    - ${f.path}`);
      }
    }

    if (opts.dryRun) {
      return;
    }

    // Confirm unless --yes is provided
    if (!opts.yes) {
      const answer = await promptInput(
        `Publish ${changes.length} change(s)? [y/N] `,
        true,
      );
      if (answer.toLowerCase() !== "y") {
        console.log("Cancelled.");
        return;
      }
    }

    console.log("Publishing...");
    await engine.apply(changes, (progress, total) => {
      process.stdout.write(`\r  Progress: ${progress}/${total}`);
    });
    process.stdout.write("\n");
    console.log("Publish complete.");
  });

/**
 * **publish-config** — Configure publish settings for a vault.
 *
 * Updates include/exclude patterns that determine which files are published.
 *
 * @example
 * ob publish-config --path ~/notes --includes "*.md" --excludes "drafts/**"
 */
program
  .command("publish-config")
  .description("Update publish configuration")
  .option("--path <path>", "Vault path", ".")
  .option("--includes <patterns>", "Comma-separated include patterns")
  .option("--excludes <patterns>", "Comma-separated exclude patterns")
  .action(async (opts) => {
    const localPath = path.resolve(opts.path);
    const config = findPublishConfigByPath(localPath);

    if (!config) {
      console.error(
        `No publish configuration found for path "${localPath}". Run "${PROGRAM_NAME} publish-setup" first.`,
      );
      process.exit(1);
    }

    let changed = false;

    if (opts.includes !== undefined) {
      config.includes = opts.includes
        .split(",")
        .map((p: string) => p.trim())
        .filter(Boolean);
      changed = true;
    }
    if (opts.excludes !== undefined) {
      config.excludes = opts.excludes
        .split(",")
        .map((p: string) => p.trim())
        .filter(Boolean);
      changed = true;
    }

    if (!changed) {
      console.log("No changes specified. Current configuration:");
      printPublishConfig(config);
      return;
    }

    writePublishConfig(config.siteId, config);
    console.log("Publish configuration updated.");
    printPublishConfig(config);
  });

/**
 * **publish-unlink** — Disconnect a local vault from a publish site.
 *
 * Removes the publish configuration. Does not delete the site or remote content.
 */
program
  .command("publish-unlink")
  .description("Disconnect a vault from publish")
  .option("--path <path>", "Vault path", ".")
  .action(async (opts) => {
    const localPath = path.resolve(opts.path);
    const config = findPublishConfigByPath(localPath);

    if (!config) {
      console.error(
        `No publish configuration found for path "${localPath}".`,
      );
      process.exit(1);
    }

    removePublishConfig(config.siteId);
    console.log(`Publish site "${config.slug ?? config.siteId}" unlinked.`);
  });

// ---------------------------------------------------------------------------
// Parse and run
// ---------------------------------------------------------------------------

program.parse();

export { program };
