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

	// Step 1: In a single pass, collect deleted UIDs and active UID→newPath mappings.
	deletedUIDs := make(map[int64]string)  // uid → oldPath
	uidToNewPath := make(map[int64]string) // uid → newPath
	for path, record := range currentRemote {
		if record.Folder || record.UID == 0 {
			continue
		}
		if record.Deleted {
			deletedUIDs[record.UID] = path
		} else {
			uidToNewPath[record.UID] = path
		}
	}

	// Guard: nothing to correlate.
	if len(deletedUIDs) == 0 {
		return result
	}

	// Step 2: Correlate deleted UIDs with active UIDs to find renames.
	for uid, oldPath := range deletedUIDs {
		newPath, ok := uidToNewPath[uid]
		if !ok || newPath == oldPath {
			continue
		}

		// Remote rename detected: oldPath → newPath
		logger.Info().
			Str("oldPath", oldPath).
			Str("newPath", newPath).
			Int64("uid", uid).
			Msg("remote rename detected via UID match")

		// Step 2a: Handle local file at oldPath
		oldLocal, hasOldLocal := currentLocal[oldPath]
		renameEnacted := false
		if hasOldLocal {
			localPath := filepath.Join(vaultPath, oldPath)
			newLocalPath := filepath.Join(vaultPath, newPath)

			// Check if local file was modified
			localModified := false
			_, hasPrevLocal := previousLocal[oldPath]
			if hasPrevLocal {
				localModified = oldLocal.Hash != previousLocal[oldPath].Hash
			}

			// If neither previousLocal nor previousRemote has oldPath, can't verify
			// the local file corresponds to the remote file — treat as conflict.
			if !hasPrevLocal {
				if _, hasPrevRemote := previousRemote[oldPath]; !hasPrevRemote {
					logger.Warn().
						Str("oldPath", oldPath).
						Msg("remote rename: no previous state for oldPath, preserving local file")
					result.Conflicts = append(result.Conflicts, oldPath)
					continue
				}
			}

			if localModified {
				// Local file was modified → preserve it, don't rename
				logger.Warn().
					Str("oldPath", oldPath).
					Str("localHash", oldLocal.Hash).
					Str("prevHash", previousLocal[oldPath].Hash).
					Msg("remote rename: local file modified, preserving")
				result.Conflicts = append(result.Conflicts, oldPath)
			} else if _, exists := currentLocal[newPath]; exists {
				// Destination already exists locally
				logger.Warn().
					Str("oldPath", oldPath).
					Str("newPath", newPath).
					Msg("remote rename: destination exists locally, preserving")
				result.Conflicts = append(result.Conflicts, oldPath)
			} else {
			// Local file is unmodified and destination is clear → rename it on disk.
			// Register the ignore pair before the filesystem rename to prevent
			// the watcher from seeing our own rename as a user-initiated change.
			pair := model.RenamePair{OldPath: oldPath, NewPath: newPath}
			if onBeforeRename != nil {
				onBeforeRename(pair)
			}
			if err := os.MkdirAll(filepath.Dir(newLocalPath), 0o755); err != nil {
				logger.Error().
					Err(err).
					Str("newPath", newPath).
					Msg("remote rename: os.MkdirAll failed, treating as conflict")
				result.Conflicts = append(result.Conflicts, oldPath)
			} else if err := os.Rename(localPath, newLocalPath); err != nil {
				logger.Error().
					Err(err).
					Str("oldPath", oldPath).
					Str("newPath", newPath).
					Msg("remote rename: os.Rename failed, treating as conflict")
				result.Conflicts = append(result.Conflicts, oldPath)
			} else {
				// Rename succeeded on disk
				oldLocal.Path = newPath
				currentLocal[newPath] = oldLocal
				delete(currentLocal, oldPath)

				result.Enacted = append(result.Enacted, pair)
				renameEnacted = true
				logger.Info().
					Str("oldPath", oldPath).
					Str("newPath", newPath).
					Msg("remote rename: local file renamed on disk")
			}
			}
		}
		// else: local file already absent → nothing to rename on disk

		// Step 2b: Update metadata state — only if the rename was enacted on disk
		// or the local file was already absent (rename happened on another device).
		if renameEnacted || !hasOldLocal {
			// Update previousRemote to reflect the rename
			if oldRemote, exists := previousRemote[oldPath]; exists {
				oldRemote.PreviousPath = oldPath
				oldRemote.Path = newPath
				previousRemote[newPath] = oldRemote
				delete(previousRemote, oldPath)
			}

			// Remove the deleted entry from currentRemote so buildPlan
			// doesn't emit a deleteLocal action for oldPath.
			delete(currentRemote, oldPath)
		}
		// If !renameEnacted && hasOldLocal: local file exists but rename was
		// blocked (modified, dest exists, or rename failed). Don't mutate
		// previousRemote or currentRemote — let buildPlan handle both paths
		// independently.
	}

	return result
}
