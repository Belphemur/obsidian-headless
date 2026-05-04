package sync

import (
	"os"
	"path/filepath"

	"github.com/rs/zerolog"

	"github.com/Belphemur/obsidian-headless/internal/model"
	watchpkg "github.com/Belphemur/obsidian-headless/internal/sync/watch"
)

// RemoteRenameResult holds the outcome of applyRemoteRenameFixups.
// Conflicts lists paths of locally modified files that were preserved (not renamed).
// Enacted lists the rename operations that were successfully performed on disk.
type RemoteRenameResult struct {
	Conflicts []string
	Enacted   []watchpkg.RenamePair
}

// applyRemoteRenameFixups detects remote renames by correlating deleted and active
// entries in currentRemote that share the same UID. When a rename is found:
//  1. previousRemote is mutated to reflect the rename (PreviousPath set, record moved)
//  2. The local file at oldPath is os.Rename'd to newPath if it exists and is unmodified
//  3. The deleted entry is removed from currentRemote
//
// Returns (nil, error) only on fatal error (e.g. os.Rename failure).
// Returns (*RemoteRenameResult, nil) on success with possible Conflicts.
func applyRemoteRenameFixups(
	currentRemote map[string]model.FileRecord,
	previousRemote map[string]model.FileRecord,
	previousLocal map[string]model.FileRecord,
	currentLocal map[string]model.FileRecord,
	vaultPath string,
	logger zerolog.Logger,
) (*RemoteRenameResult, error) {
	result := &RemoteRenameResult{}

	// Step 1: Scan for deleted entries with UIDs in currentRemote
	deletedUIDs := make(map[int64]string) // uid → oldPath
	for path, record := range currentRemote {
		if !record.Deleted || record.Folder {
			continue
		}
		if record.UID == 0 {
			continue
		}
		deletedUIDs[record.UID] = path
	}

	// Guard: no deleted entries to correlate
	if len(deletedUIDs) == 0 {
		return result, nil
	}

	// Step 2: Build UID→newPath map from active entries in currentRemote
	uidToNewPath := make(map[int64]string) // uid → newPath
	for path, record := range currentRemote {
		if record.Deleted || record.Folder {
			continue
		}
		if record.UID == 0 {
			continue
		}
		uidToNewPath[record.UID] = path
	}

	// Step 3: Correlate deleted UIDs with active UIDs to find renames
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

		// Step 3a: Handle local file at oldPath
		oldLocal, hasOldLocal := currentLocal[oldPath]
		renameEnacted := false
		if hasOldLocal {
			localPath := filepath.Join(vaultPath, oldPath)
			newLocalPath := filepath.Join(vaultPath, newPath)

			// Check if local file was modified
			localModified := false
			if prevLocal, hasPrevLocal := previousLocal[oldPath]; hasPrevLocal {
				localModified = oldLocal.Hash != prevLocal.Hash
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
				// Local file is unmodified and destination is clear → rename it on disk
				if err := os.Rename(localPath, newLocalPath); err != nil {
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

					result.Enacted = append(result.Enacted, watchpkg.RenamePair{OldPath: oldPath, NewPath: newPath})
					renameEnacted = true
					logger.Info().
						Str("oldPath", oldPath).
						Str("newPath", newPath).
						Msg("remote rename: local file renamed on disk")
				}
			}
		}
		// else: local file already absent → nothing to rename on disk

		// Step 3b: Update metadata state — only if the rename was enacted on disk
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

	return result, nil
}
