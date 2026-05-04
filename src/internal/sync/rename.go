package sync

import (
	"os"
	"path/filepath"

	"github.com/rs/zerolog"

	"github.com/Belphemur/obsidian-headless/internal/model"
)

// RemoteRenameResult holds the outcome of applyRemoteRenameFixups.
// Conflicts lists paths of locally modified files that were preserved (not renamed).
// Enacted lists the rename operations that were successfully performed on disk.
type RemoteRenameResult struct {
	Conflicts []string
	Enacted   []model.RenamePair
}

// applyRemoteRenameFixups detects remote renames by correlating deleted and active
// entries in currentRemote that share the same UID. When a rename is found:
//  1. onBeforeRename is called (if non-nil) to allow the caller to suppress watcher
//     events for oldPath and newPath before the filesystem rename occurs.
//  2. previousRemote is mutated to reflect the rename (PreviousPath set, record moved)
//  3. The local file at oldPath is os.Rename'd to newPath if it exists and is unmodified
//  4. The deleted entry is removed from currentRemote
//
// Returns *RemoteRenameResult — the function never returns an error.
// Rename failures are recorded as Conflicts in the result rather than returned as errors.
func applyRemoteRenameFixups(
	currentRemote map[string]model.FileRecord,
	previousRemote map[string]model.FileRecord,
	previousLocal map[string]model.FileRecord,
	currentLocal map[string]model.FileRecord,
	vaultPath string,
	logger zerolog.Logger,
	onBeforeRename func(model.RenamePair),
) *RemoteRenameResult {
	result := &RemoteRenameResult{}

	// Debug: log UIDs of all deleted and active records for diagnostics.
	if logger.Debug().Enabled() {
		for path, record := range currentRemote {
			if record.Folder {
				continue
			}
			logger.Debug().
				Str("path", path).
				Int64("uid", record.UID).
				Bool("deleted", record.Deleted).
				Msg("remote rename fixups: currentRemote record")
		}
	}

	// Guard: nothing to do if there are no deleted records.
	hasDeletedRecords := false
	for _, record := range currentRemote {
		if !record.Folder && record.Deleted {
			hasDeletedRecords = true
			break
		}
	}
	if !hasDeletedRecords {
		return result
	}

	// Step 1: In a single pass, collect deleted UIDs, active UID→newPath
	// mappings, and a list of deleted paths for the hash fallback.
	deletedUIDs := make(map[int64]string)  // uid → oldPath
	uidToNewPath := make(map[int64]string) // uid → newPath
	var deletedPaths []string
	for path, record := range currentRemote {
		if record.Folder {
			continue
		}
		if record.Deleted {
			deletedPaths = append(deletedPaths, path)
		}
		if record.UID == 0 {
			continue
		}
		if record.Deleted {
			deletedUIDs[record.UID] = path
		} else {
			uidToNewPath[record.UID] = path
		}
	}

	// processedPaths tracks deleted records that were examined by UID matching,
	// so the hash fallback can skip them.
	processedPaths := make(map[string]struct{})

	// uidMatchedActivePaths tracks active paths that were targeted by
	// UID-based or hash-based renames, preventing them from being reused.
	uidMatchedActivePaths := make(map[string]struct{})

	// Step 2: Correlate deleted UIDs with active UIDs to find renames.
	for uid, oldPath := range deletedUIDs {
		newPath, ok := uidToNewPath[uid]
		if !ok || newPath == oldPath {
			continue
		}
		processedPaths[oldPath] = struct{}{}

		logger.Info().
			Str("oldPath", oldPath).
			Str("newPath", newPath).
			Int64("uid", uid).
			Msg("remote rename detected via UID match")

		uidMatchedActivePaths[newPath] = struct{}{}
		enacted, conflict := handleRemoteRename(oldPath, newPath, currentRemote, previousRemote, previousLocal, currentLocal, vaultPath, logger, onBeforeRename)
		if conflict != "" {
			result.Conflicts = append(result.Conflicts, conflict)
			continue
		}
		if enacted {
			result.Enacted = append(result.Enacted, model.RenamePair{OldPath: oldPath, NewPath: newPath})
		}
	}

	// Step 3: Hash-based fallback for deleted records not matched by UID.
	// Build a map of hash → active paths from the remaining active records.
	hashToActivePaths := make(map[string][]string)
	for path, record := range currentRemote {
		if record.Folder || record.Deleted || record.Hash == "" {
			continue
		}
		hashToActivePaths[record.Hash] = append(hashToActivePaths[record.Hash], path)
	}

	for _, path := range deletedPaths {
		if _, processed := processedPaths[path]; processed {
			continue
		}
		record := currentRemote[path]

		deletedHash := record.Hash
		if deletedHash == "" {
			if prev, ok := previousRemote[path]; ok {
				deletedHash = prev.Hash
			}
		}
		if deletedHash == "" {
			continue
		}

		activePaths, ok := hashToActivePaths[deletedHash]
		if !ok {
			continue
		}

		var candidates []string
		for _, activePath := range activePaths {
			if activePath == path {
				continue
			}
			if _, consumed := uidMatchedActivePaths[activePath]; consumed {
				continue
			}
			candidates = append(candidates, activePath)
		}

		if len(candidates) == 0 {
			continue
		}
		if len(candidates) > 1 {
			logger.Warn().
				Str("oldPath", path).
				Str("hash", deletedHash).
				Int("matchCount", len(candidates)).
				Msg("remote rename: ambiguous hash match, skipping")
			continue
		}

		newPath := candidates[0]

		logger.Info().
			Str("oldPath", path).
			Str("newPath", newPath).
			Str("hash", deletedHash).
			Msg("remote rename detected via hash match")

		uidMatchedActivePaths[newPath] = struct{}{}
		enacted, conflict := handleRemoteRename(path, newPath, currentRemote, previousRemote, previousLocal, currentLocal, vaultPath, logger, onBeforeRename)
		if conflict != "" {
			result.Conflicts = append(result.Conflicts, conflict)
			continue
		}
		if enacted {
			result.Enacted = append(result.Enacted, model.RenamePair{OldPath: path, NewPath: newPath})
		}
	}

	return result
}

// handleRemoteRename performs the local filesystem rename and metadata updates
// for a detected remote rename from oldPath to newPath.
// Returns true if the local file was renamed on disk, and a non-empty conflict
// path if the rename was blocked (local modified, destination exists, etc.).
func handleRemoteRename(
	oldPath, newPath string,
	currentRemote map[string]model.FileRecord,
	previousRemote map[string]model.FileRecord,
	previousLocal map[string]model.FileRecord,
	currentLocal map[string]model.FileRecord,
	vaultPath string,
	logger zerolog.Logger,
	onBeforeRename func(model.RenamePair),
) (enacted bool, conflict string) {
	oldLocal, hasOldLocal := currentLocal[oldPath]
	renameEnacted := false
	if hasOldLocal {
		localPath := filepath.Join(vaultPath, oldPath)
		newLocalPath := filepath.Join(vaultPath, newPath)

		localModified := false
		_, hasPrevLocal := previousLocal[oldPath]
		if hasPrevLocal {
			localModified = oldLocal.Hash != previousLocal[oldPath].Hash
		}

		if !hasPrevLocal {
			if _, hasPrevRemote := previousRemote[oldPath]; !hasPrevRemote {
				logger.Warn().
					Str("oldPath", oldPath).
					Msg("remote rename: no previous state for oldPath, preserving local file")
				return false, oldPath
			}
		}

		if localModified {
			logger.Warn().
				Str("oldPath", oldPath).
				Str("localHash", oldLocal.Hash).
				Str("prevHash", previousLocal[oldPath].Hash).
				Msg("remote rename: local file modified, preserving")
			return false, oldPath
		}

		if _, exists := currentLocal[newPath]; exists {
			logger.Warn().
				Str("oldPath", oldPath).
				Str("newPath", newPath).
				Msg("remote rename: destination exists locally, preserving")
			return false, oldPath
		}

		pair := model.RenamePair{OldPath: oldPath, NewPath: newPath}
		if onBeforeRename != nil {
			onBeforeRename(pair)
		}
		if err := os.MkdirAll(filepath.Dir(newLocalPath), 0o755); err != nil {
			logger.Error().
				Err(err).
				Str("newPath", newPath).
				Msg("remote rename: os.MkdirAll failed, treating as conflict")
			return false, oldPath
		}
		if err := os.Rename(localPath, newLocalPath); err != nil {
			logger.Error().
				Err(err).
				Str("oldPath", oldPath).
				Str("newPath", newPath).
				Msg("remote rename: os.Rename failed, treating as conflict")
			return false, oldPath
		}

		oldLocal.Path = newPath
		currentLocal[newPath] = oldLocal
		delete(currentLocal, oldPath)

		renameEnacted = true
		logger.Info().
			Str("oldPath", oldPath).
			Str("newPath", newPath).
			Msg("remote rename: local file renamed on disk")
	}

	if renameEnacted || !hasOldLocal {
		if oldRemote, exists := previousRemote[oldPath]; exists {
			oldRemote.PreviousPath = oldPath
			oldRemote.Path = newPath
			previousRemote[newPath] = oldRemote
			delete(previousRemote, oldPath)
		}

		// When the server assigned a new UID (hash-fallback renames),
		// sync it into previousRemote so three-way merges can pull
		// base content by the correct UID.
		if newRecord, ok := currentRemote[newPath]; ok && newRecord.UID != 0 {
			if prev, ok := previousRemote[newPath]; ok && prev.UID != newRecord.UID {
				prev.UID = newRecord.UID
				previousRemote[newPath] = prev
			}
		}

		delete(currentRemote, oldPath)
	}

	return renameEnacted, ""
}
