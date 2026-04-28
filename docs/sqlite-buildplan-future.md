# SQL-Based BuildPlan: Future Directions

This document captures ideas for moving the sync plan computation
(`buildPlan` in `src/internal/sync/plan.go`) from in-memory Go to SQL queries.

## Current State

`buildPlan` takes four in-memory maps and produces a list of sync actions:
- `currentLocal` — filesystem scan
- `previousLocal` — loaded from `local_files` table
- `currentRemote` — WebSocket state
- `previousRemote` — loaded from `server_files` table

The logic is sequential per-path: hash comparison, change detection, merge
resolution, mtime-based conflict winner. All four maps are fully materialized
in RAM.

## Why Keep It in Go (for now)

1. **Crystal-clear logic.** The decision tree (both-sides-changed, only-remote,
   only-local) is ~70 lines of Go with explicit conditionals. Equivalent SQL
   would be a multi-level CASE with LEFT JOINs across 4 sources — harder to
   maintain and test.

2. **In-memory is fast.** At 2,000 files, the entire plan computation takes
   microseconds. Even at 100,000 files, it's tens of milliseconds.

3. **Temp table overhead.** To do plan computation in SQL, `currentLocal` and
   `currentRemote` must be inserted into temp tables first. For small syncs,
   the INSERT overhead exceeds the plan cost.

## When to Revisit

SQL-based planning becomes valuable when:

- Vault sizes exceed 50,000–100,000 files and memory pressure is a concern
- The hash index on `local_files(hash)` and `server_files(hash)` can be
  leveraged for fast pre-filtering
- Incremental sync detection via `WHERE mtime > last_sync_time` could reduce
  the scanned set

## Possible Approaches

### Approach A: Hybrid pre-filter

Load only paths that changed (where db hash != current hash, or mtime differs)
from `local_files`, reducing the in-memory map size for large vaults.

```sql
SELECT path, hash, mtime FROM local_files
WHERE mtime > ?  -- last seen mtime
```

Then compare only those against the current filesystem scan.

### Approach B: Full SQL plan

Insert `currentLocal` and `currentRemote` into temp tables, then run a single
query producing the action list:

```sql
WITH all_paths AS (
    SELECT path FROM current_local
    UNION SELECT path FROM prev_local
    UNION SELECT path FROM current_remote
    UNION SELECT path FROM prev_remote
)
SELECT path,
    CASE
        WHEN cl.hash = cr.hash AND NOT cl.folder THEN 'skip'
        WHEN rc.local AND rc.remote THEN -- merge logic
        WHEN rc.remote THEN 'download'
        WHEN rc.local THEN 'upload'
        ...
    END AS action
FROM all_paths p
LEFT JOIN current_local cl ON p.path = cl.path
LEFT JOIN prev_local pl ON p.path = pl.path
LEFT JOIN current_remote cr ON p.path = cr.path
LEFT JOIN prev_remote pr ON p.path = pr.path;
```

**Caveats:** Merge resolution and `chooseRemote` logic involve nested
conditionals that are cumbersome in SQL. Testing becomes harder (need to
set up temp tables instead of just passing maps).

### Approach C: Streaming with partial loads

Instead of loading ALL records from `local_files`/`server_files`, use a cursor
to stream records in batches, reducing peak memory. Combine with Approach A's
pre-filter.

## Schema Readiness

The current schema is ready for SQL-based approaches:

- `local_files` has indexed `hash`, `mtime`, `size` columns
- `server_files` has indexed `hash`, `uid` columns
- `raw` (JSONB) preserves all original fields for potential `json_extract` queries

## Decision

Keep `buildPlan` in Go for the foreseeable future. The typed-column schema
and indexed tables enable gradual optimization without forcing a rewrite.
