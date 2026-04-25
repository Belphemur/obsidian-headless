package sync

import (
	"sort"

	"github.com/Belphemur/obsidian-headless/src-go/internal/model"
)

type syncAction struct {
	Path string
	Kind string
}

func buildPlan(currentLocal, previousLocal, currentRemote, previousRemote, deletedRemote map[string]model.FileRecord) []syncAction {
	pathsSet := map[string]struct{}{}
	for _, collection := range []map[string]model.FileRecord{currentLocal, previousLocal, currentRemote, previousRemote, deletedRemote} {
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
		isDeleted := false
		if dr, ok := deletedRemote[path]; ok && dr.Deleted {
			isDeleted = true
		}
		localChanged := recordChanged(hasPreviousL, previousL, hasCurrentL, currentL)
		remoteChanged := recordChanged(hasPreviousR, previousR, hasCurrentR, currentR)
		if isDeleted {
			actions = append(actions, syncAction{Path: path, Kind: "delete-remote"})
			continue
		}
		switch {
		case remoteChanged && localChanged:
			if chooseRemote(hasCurrentL, currentL, hasCurrentR, currentR, hasPreviousL, previousL, hasPreviousR, previousR) {
				actions = append(actions, syncAction{Path: path, Kind: "download"})
			} else if hasCurrentL {
				actions = append(actions, syncAction{Path: path, Kind: "upload"})
			} else {
				actions = append(actions, syncAction{Path: path, Kind: "delete-remote"})
			}
		case remoteChanged:
			actions = append(actions, syncAction{Path: path, Kind: "download"})
		case localChanged:
			if hasCurrentL {
				actions = append(actions, syncAction{Path: path, Kind: "upload"})
			} else {
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
