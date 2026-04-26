package sync

import (
	"encoding/json"
	"fmt"
	"maps"
	"path/filepath"

	"github.com/sergi/go-diff/diffmatchpatch"
)

// threeWayMerge performs a diff-match-patch based three-way text merge.
// It computes the difference between base and local, turns those diffs
// into patches, and applies them to remote.
func threeWayMerge(base, local, remote string) string {
	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(base, local, true)
	diffs = dmp.DiffCleanupSemantic(diffs)
	diffs = dmp.DiffCleanupEfficiency(diffs)
	patches := dmp.PatchMake(base, diffs)
	merged, _ := dmp.PatchApply(patches, remote)
	return merged
}

// jsonMerge merges two JSON objects. Server keys win on conflict.
func jsonMerge(localJSON, remoteJSON string) (string, error) {
	var localObj, remoteObj map[string]any
	if err := json.Unmarshal([]byte(localJSON), &localObj); err != nil {
		return "", fmt.Errorf("invalid local JSON: %w", err)
	}
	if err := json.Unmarshal([]byte(remoteJSON), &remoteObj); err != nil {
		return "", fmt.Errorf("invalid remote JSON: %w", err)
	}
	merged := make(map[string]any, len(localObj)+len(remoteObj))
	maps.Copy(merged, localObj)
	maps.Copy(merged, remoteObj)
	out, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal merged JSON: %w", err)
	}
	return string(out), nil
}

// isMergeablePath returns true if the file should use merge conflict
// resolution instead of last-write-wins when both sides changed.
func isMergeablePath(path string) bool {
	ext := filepath.Ext(path)
	return ext == ".md"
}

// isJSONConfigPath returns true if the file is a JSON file inside the
// config directory and should use JSON object-key merging.
func isJSONConfigPath(path, configDir string) bool {
	if filepath.Ext(path) != ".json" {
		return false
	}
	if configDir == "" {
		configDir = ".obsidian"
	}
	return filepath.Dir(path) == configDir
}
