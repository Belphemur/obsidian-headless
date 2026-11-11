package sync

import (
	"sort"

	"github.com/Belphemur/obsidian-headless/internal/model"
	"github.com/Belphemur/obsidian-headless/internal/util"
)

type syncActionKind int

const (
	syncActionDownload syncActionKind = iota
	syncActionUpload
	syncActionDeleteRemote
	syncActionDeleteLocal
	syncActionMergeText
	syncActionMergeJSON
)

func (k syncActionKind) String() string {
	switch k {
	case syncActionDownload:
		return "download"
	case syncActionUpload:
		return "upload"
	case syncActionDeleteRemote:
		return "delete-remote"
	case syncActionDeleteLocal:
		return "delete-local"
	case syncActionMergeText:
		return "merge-text"
	case syncActionMergeJSON:
		return "merge-json"
	default:
		return "unknown"
	}
}

type syncAction struct {
	Path        string
	Kind        syncActionKind
	RelatedPath string // old path for rename uploads; empty for other actions
}

// Sync modes, as stored in model.SyncConfig.SyncMode. The empty string and
// "bidirectional" both mean two-way sync.
const (
	syncModePull   = "pull"
	syncModeMirror = "mirror"
)

// isReadOnlyMode reports whether a sync mode must never change the remote vault.
func isReadOnlyMode(mode string) bool {
	return mode == syncModePull || mode == syncModeMirror
}

// buildPlan plans a bidirectional sync.
func buildPlan(
	currentLocal, previousLocal,
	currentRemote, previousRemote map[string]model.FileRecord,
	configDir string,
	localRenames map[string]string, // oldPath → newPath from watcher snapshot
) []syncAction {
	return buildPlanForMode("", currentLocal, previousLocal, currentRemote, previousRemote, configDir, localRenames)
}

// buildPlanForMode plans a sync for the given mode.
//
// In the read-only modes the plan only ever contains downloads and local
// deletions, so the remote vault is never modified:
//
//   - pull: remote changes are downloaded and local changes are ignored (kept
//     on disk, never uploaded). When both sides changed, the server wins.
//   - mirror: as pull, and local changes are reverted: a locally modified or
//     deleted file is downloaded again and a file that is not on the server is
//     deleted locally.
func buildPlanForMode(
	mode string,
	currentLocal, previousLocal,
	currentRemote, previousRemote map[string]model.FileRecord,
	configDir string,
	localRenames map[string]string, // oldPath → newPath from watcher snapshot
) []syncAction {
	readOnly := isReadOnlyMode(mode)
	if readOnly {
		// A local rename is a local change like any other: it is never sent.
		localRenames = nil
	}
	pathsSet := map[string]struct{}{}
	for _, collection := range []map[string]model.FileRecord{currentLocal, previousLocal, currentRemote, previousRemote} {
		for path := range collection {
			if isValidPath(path) {
				pathsSet[path] = struct{}{}
			}
		}
	}
	paths := make([]string, 0, len(pathsSet))
	for path := range pathsSet {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	actions := make([]syncAction, 0, len(paths))

	// Build reverse lookup: newPath → oldPath for O(1) checks
	renameTargets := make(map[string]string, len(localRenames))
	for oldPath, newPath := range localRenames {
		renameTargets[newPath] = oldPath
	}

	for _, path := range paths {
		// Skip paths that were renamed away — they are not deletes,
		// but only when the rename target still exists locally.
		// If the target was deleted before this sync cycle, fall through
		// so the planner can issue a delete-remote for the orphaned path.
		if newPath, wasRenamedAway := localRenames[path]; wasRenamedAway {
			if _, exists := currentLocal[newPath]; exists {
				continue
			}
		}

		currentL, hasCurrentL := currentLocal[path]
		previousL, hasPreviousL := previousLocal[path]
		currentR, hasCurrentR := currentRemote[path]
		previousR, hasPreviousR := previousRemote[path]

		// If this path is a rename target, force an upload with relatedpath
		// so the server receives the rename as a single operation.
		if oldPath, isRenameTarget := renameTargets[path]; isRenameTarget && hasCurrentL {
			actions = append(actions, syncAction{
				Path:        path,
				Kind:        syncActionUpload,
				RelatedPath: oldPath,
			})
			continue
		}

		// Hash match: no changes needed
		if hasCurrentL && hasCurrentR && !currentL.Folder && currentL.Hash == currentR.Hash {
			continue
		}

		localChanged := recordChanged(hasPreviousL, previousL, hasCurrentL, currentL)
		remoteChanged := recordChanged(hasPreviousR, previousR, hasCurrentR, currentR)

		// Server has an active (non-deleted) file
		serverHasActiveFile := hasCurrentR && !currentR.Deleted
		// Server has a deleted record for this file
		serverHasDeletedRecord := hasCurrentR && currentR.Deleted

		switch {
		case remoteChanged && localChanged && readOnly:
			// Pull and mirror: the server always wins.
			if serverHasActiveFile {
				actions = append(actions, syncAction{Path: path, Kind: syncActionDownload})
			} else if serverHasDeletedRecord && hasCurrentL {
				actions = append(actions, syncAction{Path: path, Kind: syncActionDeleteLocal})
			}
		case localChanged && !remoteChanged && readOnly:
			// Pull ignores local changes. Mirror reverts them.
			if mode == syncModeMirror {
				if serverHasActiveFile {
					// Modified or deleted locally: take the server's version again.
					actions = append(actions, syncAction{Path: path, Kind: syncActionDownload})
				} else if hasCurrentL && !currentL.Folder {
					// Not on the server: remove it. Folders are left alone, as
					// they may still hold files this cycle.
					actions = append(actions, syncAction{Path: path, Kind: syncActionDeleteLocal})
				}
			}
		case remoteChanged && localChanged:
			if serverHasDeletedRecord {
				// Server deleted, local changed - let local win (upload)
				if hasCurrentL {
					actions = append(actions, syncAction{Path: path, Kind: syncActionUpload})
				}
				break
			}
			if !serverHasActiveFile {
				// Neither side has an active file, nothing to do
				break
			}
			if hasCurrentL {
				// Both sides have active changes. Use merge for supported types,
				// otherwise fall back to mtime-based winner.
				if isMergeablePath(path) {
					actions = append(actions, syncAction{Path: path, Kind: syncActionMergeText})
				} else if isJSONConfigPath(path, configDir) {
					actions = append(actions, syncAction{Path: path, Kind: syncActionMergeJSON})
				} else if chooseRemote(hasCurrentL, currentL, hasCurrentR, currentR, hasPreviousL, previousL, hasPreviousR, previousR) {
					actions = append(actions, syncAction{Path: path, Kind: syncActionDownload})
				} else {
					actions = append(actions, syncAction{Path: path, Kind: syncActionUpload})
				}
			} else {
				// Local deleted, remote changed - fall back to mtime-based winner.
				if chooseRemote(hasCurrentL, currentL, hasCurrentR, currentR, hasPreviousL, previousL, hasPreviousR, previousR) {
					actions = append(actions, syncAction{Path: path, Kind: syncActionDownload})
				} else if serverHasActiveFile {
					actions = append(actions, syncAction{Path: path, Kind: syncActionDeleteRemote})
				}
			}
		case remoteChanged:
			if serverHasActiveFile {
				actions = append(actions, syncAction{Path: path, Kind: syncActionDownload})
			} else if serverHasDeletedRecord && hasCurrentL {
				// Server deleted the file and local hasn't changed - delete local copy
				actions = append(actions, syncAction{Path: path, Kind: syncActionDeleteLocal})
			}
		case localChanged:
			if hasCurrentL {
				actions = append(actions, syncAction{Path: path, Kind: syncActionUpload})
			} else if serverHasActiveFile {
				actions = append(actions, syncAction{Path: path, Kind: syncActionDeleteRemote})
			}
		}
	}
	return actions
}

// localBaselineAfterReadOnlySync returns the local state to remember after a
// sync in a read-only mode.
//
// The planner compares the files on disk with this baseline to tell what
// changed locally. A local change that a read-only plan ignored (an edit, a
// deletion or a new file in pull mode) must therefore not enter the baseline:
// it stays pending, so that it is still seen as a local change in later
// cycles and is synced once the mode is set back to bidirectional.
//
// plan is the plan that was just executed, previousLocal the baseline it was
// built against, currentLocal the files on disk after executing it and remote
// the remote records after it.
func localBaselineAfterReadOnlySync(plan []syncAction, previousLocal, currentLocal, remote map[string]model.FileRecord) map[string]model.FileRecord {
	// Paths the plan brought in line with the server (downloaded or deleted locally).
	reconciled := make(map[string]struct{}, len(plan))
	for _, action := range plan {
		reconciled[action.Path] = struct{}{}
	}
	baseline := make(map[string]model.FileRecord, len(currentLocal))
	for path, current := range currentLocal {
		previous, hadPrevious := previousLocal[path]
		_, wasReconciled := reconciled[path]
		remoteRecord, hasRemote := remote[path]
		matchesServer := hasRemote && !remoteRecord.Deleted && !current.Folder && current.Hash == remoteRecord.Hash
		if wasReconciled || matchesServer || !recordChanged(hadPrevious, previous, true, current) {
			baseline[path] = current
			continue
		}
		// An ignored local edit keeps its old record; an ignored new file gets none.
		if hadPrevious {
			baseline[path] = previous
		}
	}
	// An ignored local deletion keeps its old record.
	for path, previous := range previousLocal {
		if _, onDisk := currentLocal[path]; onDisk {
			continue
		}
		if _, wasReconciled := reconciled[path]; wasReconciled {
			continue
		}
		baseline[path] = previous
	}
	return baseline
}

func recordChanged(hadBefore bool, before model.FileRecord, hasNow bool, now model.FileRecord) bool {
	if hadBefore != hasNow {
		return true
	}
	if !hadBefore && !hasNow {
		return false
	}
	return before.Hash != now.Hash || before.MTime != now.MTime || before.Size != now.Size || before.Deleted != now.Deleted
}

func chooseRemote(hasCurrentL bool, currentL model.FileRecord, hasCurrentR bool, currentR model.FileRecord, hasPreviousL bool, previousL model.FileRecord, hasPreviousR bool, previousR model.FileRecord) bool {
	// If remote file is deleted, local wins
	if hasCurrentR && currentR.Deleted {
		return false
	}
	localTime := int64(0)
	remoteTime := int64(0)
	if hasCurrentL {
		localTime = currentL.MTime
	} else if hasPreviousL {
		localTime = previousL.MTime
	}
	if hasCurrentR {
		remoteTime = currentR.MTime
	} else if hasPreviousR {
		remoteTime = previousR.MTime
	}
	if remoteTime == localTime {
		return hasCurrentR && (!hasCurrentL || currentR.Hash != currentL.Hash)
	}
	return remoteTime > localTime
}

func isValidPath(path string) bool {
	if path == "" {
		return false
	}
	if !util.IsLegalPath(path) {
		return false
	}
	if len(path) > 3 && len(path) < 4096 {
		return true
	}
	return len(path) >= 3
}
