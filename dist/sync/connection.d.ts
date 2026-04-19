/**
 * @module sync/connection
 *
 * Manages the WebSocket connection to the Obsidian Sync server.
 * Handles authentication, heartbeat keep-alive, request/response
 * serialization, and server push notifications.
 */
import { AsyncQueue, type Deferred } from "../utils/async.js";
import type { EncryptionProvider } from "../encryption/types.js";
/** A file record pushed from the server or returned by list operations. */
export interface ServerPushFile {
    path: string;
    hash: string;
    ctime: number;
    mtime: number;
    size: number;
    folder: boolean;
    deleted: boolean;
    uid: number;
    device: string;
    user: string;
    /** Set to `true` when this push originated from us in the current session. */
    wasJustPushed?: boolean;
}
/**
 * Manages a single WebSocket connection to the Obsidian Sync server.
 *
 * All request/response pairs are serialized through an {@link AsyncQueue}
 * to prevent interleaving. Server push notifications are processed in a
 * separate queue so they do not block request flow.
 */
export declare class SyncServerConnection {
    /** The active WebSocket connection. */
    socket: WebSocket | null;
    /** Serializes all request/response operations. */
    queue: AsyncQueue;
    /** Processes server push notifications. */
    notifyQueue: AsyncQueue;
    /** Buffered binary data received from the server. */
    dataQueue: ArrayBuffer[];
    /** Timestamp of the last message received from the server. */
    lastMessageTs: number;
    /** Timestamp of the last network request we initiated. */
    lastNetworkRequestTs: number;
    /** Callback invoked when the connection is lost or closed. */
    onDisconnect: (() => void) | null;
    /** Maximum file size the server accepts (bytes). */
    perFileMax: number;
    /** Authenticated user ID, set after successful init handshake. */
    userId: number;
    /** Tracks the most recently pushed file to detect self-echoes. */
    justPushed: {
        path: string;
        folder: boolean;
        deleted: boolean;
        mtime: number;
        hash: string;
    } | null;
    /** Encryption provider for path encoding and content encryption. */
    encryptionProvider: EncryptionProvider;
    /** Handle for the heartbeat interval timer. */
    heartbeat: ReturnType<typeof setInterval> | null;
    /** Pending response promise for the current request. */
    responsePromise: Deferred<any> | null;
    /** Pending binary data promise. */
    dataPromise: Deferred<ArrayBuffer> | null;
    /** Callback invoked when the server signals readiness with a version. */
    onReady: ((version: number) => void) | null;
    /** Callback invoked when the server pushes a file change notification. */
    onPush: ((file: ServerPushFile) => void) | null;
    constructor(encryptionProvider: EncryptionProvider);
    /** Whether the socket is currently in the CONNECTING state. */
    isConnecting(): boolean;
    /** Whether the socket is currently open and ready for communication. */
    isConnected(): boolean;
    /** Whether a socket instance exists (may be connecting, open, or closing). */
    hasConnection(): boolean;
    /**
     * Establish a WebSocket connection and authenticate with the sync server.
     *
     * @param url - WebSocket server URL (wss://...)
     * @param token - Authentication token
     * @param vaultId - Remote vault identifier
     * @param version - Last known sync version number
     * @param initial - Whether this is the initial sync
     * @param deviceName - Name of this device
     * @param onReady - Callback when server sends "ready" with new version
     * @param onPush - Callback when server pushes a file change
     */
    connect(url: string, token: string, vaultId: string, version: number, initial: boolean, deviceName: string, onReady: (version: number) => void, onPush: (file: ServerPushFile) => void): Promise<void>;
    /** Close the connection and clean up all pending state. */
    disconnect(): void;
    /**
     * Pull file content from the server by UID.
     *
     * @param uid - The unique file identifier on the server
     * @returns Decrypted file content, or `null` if the file was deleted
     */
    pull(uid: number): Promise<ArrayBuffer | null>;
    /**
     * Push a file (or deletion/folder event) to the server.
     *
     * @param path - Vault-relative file path
     * @param relatedPath - Related path (e.g., rename source)
     * @param isFolder - Whether this entry is a folder
     * @param isDeleted - Whether this is a deletion event
     * @param ctime - Creation time (ms since epoch)
     * @param mtime - Modification time (ms since epoch)
     * @param hash - Content hash of the file
     * @param data - File content (null for folders/deletions)
     */
    push(path: string, relatedPath: string | null, isFolder: boolean, isDeleted: boolean, ctime: number, mtime: number, hash: string, data: ArrayBuffer | null): Promise<void>;
    /**
     * Retrieve the list of deleted files from the server.
     *
     * @returns Array of deleted file records with decrypted paths
     */
    listDeleted(): Promise<ServerPushFile[]>;
    /**
     * Retrieve the version history for a specific file.
     *
     * @param path - Vault-relative file path
     * @param lastUid - Only return versions after this UID
     * @returns Object containing an array of history items
     */
    listHistory(path: string, lastUid: number): Promise<{
        items: ServerPushFile[];
    }>;
    /** Restore a deleted file by UID. */
    restore(uid: number): Promise<any>;
    /** Get all usernames associated with this vault. */
    getUsernames(): Promise<any>;
    /** Purge all vault data from the server. */
    purge(): Promise<any>;
    /** Get total vault storage size on the server. */
    size(): Promise<any>;
    /**
     * Handle a server push notification for a file change.
     * Detects self-echoes (files we just pushed) and marks them accordingly.
     */
    onServerPush(msg: any): void;
    /**
     * Create a deferred promise for the next JSON response from the server.
     */
    response(): Promise<any>;
    /**
     * Wait for the next binary data message.
     * Returns immediately if data is already buffered.
     */
    dataResponse(): Promise<ArrayBuffer>;
    /**
     * Send a request and await the response with a timeout.
     *
     * @param msg - Message object to send
     * @param timeout - Maximum time to wait for response (ms)
     * @returns Parsed server response
     */
    request(msg: Record<string, any>, timeout?: number): Promise<any>;
    /**
     * Handle incoming WebSocket messages after the handshake is complete.
     */
    onMessage(event: MessageEvent): void;
    /** Serialize and send a JSON message. */
    send(msg: Record<string, any>): void;
    /** Send raw binary data. */
    sendBinary(data: ArrayBuffer): void;
    /** Start the keep-alive heartbeat interval. */
    private startHeartbeat;
    /** Stop the heartbeat interval. */
    private stopHeartbeat;
    /** Handle unexpected disconnection. */
    private handleDisconnect;
}
//# sourceMappingURL=connection.d.ts.map