/**
 * @module publish/engine
 *
 * The publish engine scans for local file changes and uploads/removes files
 * from the Obsidian Publish service.  It maintains a local cache of file
 * hashes and metadata to enable efficient incremental publishes.
 */
import { type PublishConfig } from "../config/index.js";
/** Describes a single file change detected during a scan. */
export interface PublishChange {
    path: string;
    type: "new" | "changed" | "deleted";
}
/**
 * Coordinates scanning, uploading, and removing files for Obsidian Publish.
 *
 * @example
 * ```ts
 * const engine = new PublishEngine(token, config);
 * const changes = await engine.scanForChanges();
 * await engine.publish(changes, (cur, total) => console.log(`${cur}/${total}`));
 * ```
 */
export declare class PublishEngine {
    private token;
    private config;
    private cache;
    constructor(token: string, config: PublishConfig);
    /**
     * Scans the local vault and compares it with the server file list to
     * determine which files need to be published, updated, or removed.
     *
     * @param includeAll - When true, publish all supported files regardless
     *   of frontmatter or folder rules.
     * @returns A list of changes to apply.
     */
    scanForChanges(includeAll?: boolean): Promise<PublishChange[]>;
    /**
     * Uploads a single file to the publish site.
     *
     * @param filePath - Vault-relative path to the file.
     * @throws If the file exceeds the 50 MB upload limit.
     */
    uploadFile(filePath: string): Promise<void>;
    /**
     * Removes a file from the publish site.
     *
     * @param filePath - Vault-relative path to the file.
     */
    removeFile(filePath: string): Promise<void>;
    /**
     * Applies a list of changes by uploading or removing files.
     *
     * @param changes - The list of changes to apply.
     * @param onProgress - Optional callback invoked after each file is processed.
     */
    publish(changes: PublishChange[], onProgress?: (current: number, total: number) => void): Promise<void>;
    /**
     * Recursively walks the vault directory and returns all publishable files
     * with their metadata.
     */
    walkLocalFiles(): Array<{
        path: string;
        mtime: number;
        size: number;
    }>;
    private walkDir;
    /**
     * Checks if a file is eligible for publishing based on its name or extension.
     */
    isFileSupported(filename: string, filePath: string): boolean;
    /**
     * Determines the publish flag based on folder include/exclude rules.
     *
     * @returns `false` if excluded, `true` if included, `null` if no rule matches.
     */
    getFolderPublishFlag(filePath: string): boolean | null;
    /**
     * Parses the YAML frontmatter of a Markdown file to extract the `publish`
     * flag.
     *
     * @returns `true` if publish is enabled, `false` if explicitly disabled,
     *   or `null` if no frontmatter or no publish key.
     */
    parseFrontmatterPublishFlag(content: string): boolean | null;
    /** Loads the publish cache from disk. */
    private loadCache;
    /** Saves the publish cache to disk. */
    private saveCache;
}
//# sourceMappingURL=engine.d.ts.map