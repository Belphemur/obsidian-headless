CREATE TABLE local_files_old (path TEXT PRIMARY KEY, data TEXT NOT NULL);
CREATE TABLE server_files_old (path TEXT PRIMARY KEY, data TEXT NOT NULL);

INSERT INTO local_files_old (path, data)
SELECT
    path,
    json(raw)
FROM local_files;

INSERT INTO server_files_old (path, data)
SELECT
    path,
    json(raw)
FROM server_files;

DROP TABLE local_files;
DROP TABLE server_files;
ALTER TABLE local_files_old RENAME TO local_files;
ALTER TABLE server_files_old RENAME TO server_files;
