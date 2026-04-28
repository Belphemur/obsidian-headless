CREATE TABLE local_files_new (
    path    TEXT PRIMARY KEY,
    size    INTEGER NOT NULL DEFAULT 0,
    hash    TEXT    NOT NULL DEFAULT '',
    ctime   INTEGER NOT NULL DEFAULT 0,
    mtime   INTEGER NOT NULL DEFAULT 0,
    folder  INTEGER NOT NULL DEFAULT 0,
    deleted INTEGER NOT NULL DEFAULT 0,
    raw     BLOB    NOT NULL DEFAULT (jsonb('{}'))
);

CREATE TABLE server_files_new (
    path    TEXT PRIMARY KEY,
    size    INTEGER NOT NULL DEFAULT 0,
    hash    TEXT    NOT NULL DEFAULT '',
    ctime   INTEGER NOT NULL DEFAULT 0,
    mtime   INTEGER NOT NULL DEFAULT 0,
    folder  INTEGER NOT NULL DEFAULT 0,
    deleted INTEGER NOT NULL DEFAULT 0,
    uid     INTEGER NOT NULL DEFAULT 0,
    device  TEXT    NOT NULL DEFAULT '',
    user    TEXT    NOT NULL DEFAULT '',
    raw     BLOB    NOT NULL DEFAULT (jsonb('{}'))
);

INSERT INTO local_files_new (path, size, hash, ctime, mtime, folder, deleted, raw)
SELECT
    path,
    COALESCE(json_extract(data, '$.size'), 0),
    COALESCE(json_extract(data, '$.hash'), ''),
    COALESCE(json_extract(data, '$.ctime'), 0),
    COALESCE(json_extract(data, '$.mtime'), 0),
    COALESCE(json_extract(data, '$.folder'), 0),
    COALESCE(json_extract(data, '$.deleted'), 0),
    jsonb(data)
FROM local_files;

INSERT INTO server_files_new (path, size, hash, ctime, mtime, folder, deleted, uid, device, user, raw)
SELECT
    path,
    COALESCE(json_extract(data, '$.size'), 0),
    COALESCE(json_extract(data, '$.hash'), ''),
    COALESCE(json_extract(data, '$.ctime'), 0),
    COALESCE(json_extract(data, '$.mtime'), 0),
    COALESCE(json_extract(data, '$.folder'), 0),
    COALESCE(json_extract(data, '$.deleted'), 0),
    COALESCE(json_extract(data, '$.uid'), 0),
    COALESCE(json_extract(data, '$.device'), ''),
    COALESCE(json_extract(data, '$.user'), ''),
    jsonb(data)
FROM server_files;

DROP TABLE local_files;
DROP TABLE server_files;
ALTER TABLE local_files_new RENAME TO local_files;
ALTER TABLE server_files_new RENAME TO server_files;

CREATE INDEX IF NOT EXISTS idx_local_files_hash ON local_files(hash);
CREATE INDEX IF NOT EXISTS idx_server_files_hash ON server_files(hash);
CREATE INDEX IF NOT EXISTS idx_server_files_uid ON server_files(uid);
