package sync

import (
	"sort"

	"github.com/Belphemur/obsidian-headless/src-go/internal/model"
)

type syncAction struct {
	Path string
	Kind string
}

func buildPlan(currentLocal, previousLocal, currentRemote, previousRemote map[string]model.FileRecord) []syncAction {
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
	for _, path := range paths {
		currentL, hasCurrentL := currentLocal[path]
		previousL, hasPreviousL := previousLocal[path]
		currentR, hasCurrentR := currentRemote[path]
		previousR, hasPreviousR := previousRemote[path]
		localChanged := recordChanged(hasPreviousL, previousL, hasCurrentL, currentL)
		remoteChanged := recordChanged(hasPreviousR, previousR, hasCurrentR, currentR)

		// Server has an active (non-deleted) file
		serverHasActiveFile := hasCurrentR && !currentR.Deleted
		// Server has a deleted record for this file
		serverHasDeletedRecord := hasCurrentR && currentR.Deleted

		switch {
		case remoteChanged && localChanged:
			if chooseRemote(hasCurrentL, currentL, hasCurrentR, currentR, hasPreviousL, previousL, hasPreviousR, previousR) {
				if serverHasActiveFile {
					actions = append(actions, syncAction{Path: path, Kind: "download"})
				} else if serverHasDeletedRecord {
					// Server deleted, local changed - conflict. For now, let local win (upload)
					// TODO: implement proper conflict resolution
				}
			}
			if hasCurrentL {
				actions = append(actions, syncAction{Path: path, Kind: "upload"})
			} else if serverHasActiveFile {
				actions = append(actions, syncAction{Path: path, Kind: "delete-remote"})
			}
		case remoteChanged:
			if serverHasActiveFile {
				actions = append(actions, syncAction{Path: path, Kind: "download"})
			} else if serverHasDeletedRecord && hasCurrentL {
				// Server deleted the file and local hasn't changed - delete local copy
				actions = append(actions, syncAction{Path: path, Kind: "delete-local"})
			}
		case localChanged:
			if hasCurrentL {
				actions = append(actions, syncAction{Path: path, Kind: "upload"})
			} else if serverHasActiveFile {
				actions = append(actions, syncAction{Path: path, Kind: "delete-remote"})
			}
		}
	}
	return actions
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

type syncActionKind int

const (
	actionDownload syncActionKind = iota
	actionUpload
	actionDeleteRemote
	actionDeleteLocal
)

func (k syncActionKind) String() string {
	switch k {
	case actionDownload:
		return "download"
	case actionUpload:
		return "upload"
	case actionDeleteRemote:
		return "delete-remote"
	case actionDeleteLocal:
		return "delete-local"
	default:
		return "unknown"
	}
}

func isValidPath(path string) bool {
	if path == "" {
		return false
	}
	if len(path) > 3 && len(path) < 4096 {
		return true
	}
	return len(path) >= 3
}
