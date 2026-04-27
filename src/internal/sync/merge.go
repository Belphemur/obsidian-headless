package sync

import (
	"encoding/json"
	"fmt"
	"maps"
	"path"
	"path/filepath"
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"
)

// threeWayMerge performs a diff-match-patch based three-way text merge.
// It computes the difference between base and local, turns those diffs
// into patches, and applies them to remote.
func threeWayMerge(base, local, remote string) (string, error) {
	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(base, local, true)
	diffs = dmp.DiffCleanupSemantic(diffs)
	diffs = dmp.DiffCleanupEfficiency(diffs)
	patches := dmp.PatchMake(base, diffs)
	merged, results := dmp.PatchApply(patches, remote)
	for _, ok := range results {
		if !ok {
			return merged, fmt.Errorf("patch failed to apply")
		}
	}
	return merged, nil
}

// jsonMerge merges two JSON objects. Server keys win on conflict.
// For overlapping map values, it recurses; for scalar/array conflicts, remote wins.
func jsonMerge(localJSON, remoteJSON string) (string, error) {
	var localObj, remoteObj map[string]any
	if err := json.Unmarshal([]byte(localJSON), &localObj); err != nil {
		return "", fmt.Errorf("invalid local JSON: %w", err)
	}
	if err := json.Unmarshal([]byte(remoteJSON), &remoteObj); err != nil {
		return "", fmt.Errorf("invalid remote JSON: %w", err)
	}
	merged := deepMergeJSON(localObj, remoteObj)
	out, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal merged JSON: %w", err)
	}
	return string(out), nil
}

// deepMergeJSON merges remote into local. For overlapping map values,
// it recurses; for scalar/array conflicts, remote wins.
func deepMergeJSON(local, remote map[string]any) map[string]any {
	out := make(map[string]any, len(local)+len(remote))
	maps.Copy(out, local)
	for k, rv := range remote {
		if lv, ok := out[k]; ok {
			if lm, lok := lv.(map[string]any); lok {
				if rm, rok := rv.(map[string]any); rok {
					out[k] = deepMergeJSON(lm, rm)
					continue
				}
			}
		}
		out[k] = rv
	}
	return out
}

// isMergeablePath returns true if the file should use merge conflict
// resolution instead of last-write-wins when both sides changed.
func isMergeablePath(path string) bool {
	ext := filepath.Ext(path)
	return ext == ".md"
}

// isJSONConfigPath returns true if the file is a JSON file inside the
// config directory (or any subdirectory) and should use JSON object-key merging.
func isJSONConfigPath(pathStr, configDir string) bool {
	// Normalize to forward slashes for consistent prefix matching
	normPath := strings.ReplaceAll(pathStr, "\\", "/")
	if path.Ext(normPath) != ".json" {
		return false
	}
	if configDir == "" {
		configDir = ".obsidian"
	}
	normDir := strings.ReplaceAll(configDir, "\\", "/")
	dir := path.Dir(normPath)
	return dir == normDir || strings.HasPrefix(dir, normDir+"/")
}
