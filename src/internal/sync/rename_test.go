package sync

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"

	"github.com/Belphemur/obsidian-headless/internal/model"
)

func setupRenameTest(t *testing.T) (string, func()) {
	t.Helper()
	dir := t.TempDir()
	return dir, func() {}
}

func TestApplyRemoteRenameFixups_BasicRename(t *testing.T) {
	t.Parallel()
	vaultPath, cleanup := setupRenameTest(t)
	defer cleanup()

	// Setup: remote renamed "old.md" → "new.md"
	uid := int64(42)
	currentRemote := map[string]model.FileRecord{
		"new.md": {Path: "new.md", Hash: "abc", UID: uid, Deleted: false},
		"old.md": {Path: "old.md", Hash: "", UID: uid, Deleted: true},
	}
	previousRemote := map[string]model.FileRecord{
		"old.md": {Path: "old.md", Hash: "abc", UID: uid},
	}
	previousLocal := map[string]model.FileRecord{
		"old.md": {Path: "old.md", Hash: "abc", UID: uid},
	}
	currentLocal := map[string]model.FileRecord{
		"old.md": {Path: "old.md", Hash: "abc"},
	}

	// Create the local file so os.Rename can succeed
	localOldPath := filepath.Join(vaultPath, "old.md")
	if err := os.WriteFile(localOldPath, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	logger := zerolog.Nop()
	result := applyRemoteRenameFixups(currentRemote, previousRemote, previousLocal, currentLocal, vaultPath, logger, nil)

	// Check result
	if len(result.Enacted) != 1 {
		t.Fatalf("expected 1 enacted rename, got %d", len(result.Enacted))
	}
	if result.Enacted[0].OldPath != "old.md" || result.Enacted[0].NewPath != "new.md" {
		t.Fatalf("unexpected rename pair: %+v", result.Enacted[0])
	}
	if len(result.Conflicts) != 0 {
		t.Fatalf("expected 0 conflicts, got %d", len(result.Conflicts))
	}

	// Check previousRemote was mutated
	if _, exists := previousRemote["old.md"]; exists {
		t.Error("old.md should have been removed from previousRemote")
	}
	if prev, exists := previousRemote["new.md"]; !exists {
		t.Error("new.md should be in previousRemote")
	} else if prev.PreviousPath != "old.md" {
		t.Errorf("expected PreviousPath 'old.md', got '%s'", prev.PreviousPath)
	}

	// Check currentRemote: old path should be deleted
	if _, exists := currentRemote["old.md"]; exists {
		t.Error("old.md should have been removed from currentRemote")
	}

	// Check currentLocal: old path should be moved to new path
	if _, exists := currentLocal["old.md"]; exists {
		t.Error("old.md should have been removed from currentLocal")
	}
	if _, exists := currentLocal["new.md"]; !exists {
		t.Error("new.md should be in currentLocal")
	}

	// Check the file was actually renamed on disk
	if _, err := os.Stat(filepath.Join(vaultPath, "old.md")); !os.IsNotExist(err) {
		t.Error("old.md should not exist on disk after rename")
	}
	if _, err := os.Stat(filepath.Join(vaultPath, "new.md")); err != nil {
		t.Error("new.md should exist on disk after rename")
	}
}

func TestApplyRemoteRenameFixups_NoRename_DifferentUIDs(t *testing.T) {
	t.Parallel()
	vaultPath, cleanup := setupRenameTest(t)
	defer cleanup()

	// UIDs don't match → no rename
	currentRemote := map[string]model.FileRecord{
		"new.md": {Path: "new.md", Hash: "abc", UID: 42, Deleted: false},
		"old.md": {Path: "old.md", Hash: "", UID: 99, Deleted: true},
	}
	previousRemote := map[string]model.FileRecord{
		"old.md": {Path: "old.md", Hash: "xyz", UID: 99},
	}
	previousLocal := map[string]model.FileRecord{
		"old.md": {Path: "old.md", Hash: "xyz", UID: 99},
	}
	currentLocal := map[string]model.FileRecord{
		"old.md": {Path: "old.md", Hash: "xyz"},
	}

	logger := zerolog.Nop()
	result := applyRemoteRenameFixups(currentRemote, previousRemote, previousLocal, currentLocal, vaultPath, logger, nil)
	if len(result.Enacted) != 0 {
		t.Fatalf("expected no enacted renames, got %d", len(result.Enacted))
	}
	if len(result.Conflicts) != 0 {
		t.Fatalf("expected no conflicts, got %d", len(result.Conflicts))
	}
	// currentRemote should still have old.md (not deleted)
	if _, exists := currentRemote["old.md"]; !exists {
		t.Error("old.md should still be in currentRemote (different UIDs, not a rename)")
	}
}

func TestApplyRemoteRenameFixups_EmptyMaps(t *testing.T) {
	t.Parallel()
	vaultPath, cleanup := setupRenameTest(t)
	defer cleanup()

	logger := zerolog.Nop()
	result := applyRemoteRenameFixups(
		map[string]model.FileRecord{},
		map[string]model.FileRecord{},
		map[string]model.FileRecord{},
		map[string]model.FileRecord{},
		vaultPath,
		logger, nil,
	)
	if len(result.Enacted) != 0 {
		t.Fatalf("expected no enacted renames from empty maps")
	}
	if len(result.Conflicts) != 0 {
		t.Fatalf("expected no conflicts from empty maps")
	}
}

func TestApplyRemoteRenameFixups_LocalModified(t *testing.T) {
	t.Parallel()
	vaultPath, cleanup := setupRenameTest(t)
	defer cleanup()

	uid := int64(42)
	// Remote renamed: old.md → new.md
	currentRemote := map[string]model.FileRecord{
		"new.md": {Path: "new.md", Hash: "abc", UID: uid, Deleted: false},
		"old.md": {Path: "old.md", Hash: "", UID: uid, Deleted: true},
	}
	previousRemote := map[string]model.FileRecord{
		"old.md": {Path: "old.md", Hash: "abc", UID: uid},
	}
	// Local has a DIFFERENT hash (modified)
	previousLocal := map[string]model.FileRecord{
		"old.md": {Path: "old.md", Hash: "abc"},
	}
	currentLocal := map[string]model.FileRecord{
		"old.md": {Path: "old.md", Hash: "modified-hash"},
	}

	// Create the local file
	localOldPath := filepath.Join(vaultPath, "old.md")
	if err := os.WriteFile(localOldPath, []byte("modified content"), 0644); err != nil {
		t.Fatal(err)
	}

	logger := zerolog.Nop()
	result := applyRemoteRenameFixups(currentRemote, previousRemote, previousLocal, currentLocal, vaultPath, logger, nil)

	// Should NOT enact rename (local modified)
	if len(result.Enacted) != 0 {
		t.Fatalf("expected 0 enacted renames (local modified), got %d", len(result.Enacted))
	}
	if len(result.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(result.Conflicts))
	}
	if result.Conflicts[0] != "old.md" {
		t.Fatalf("expected conflict path 'old.md', got '%s'", result.Conflicts[0])
	}

	// old.md should still exist on disk (not renamed)
	if _, err := os.Stat(localOldPath); err != nil {
		t.Error("old.md should still exist on disk (local modified, preserved)")
	}
	// old.md should still be in currentLocal
	if _, exists := currentLocal["old.md"]; !exists {
		t.Error("old.md should still be in currentLocal")
	}
}

func TestApplyRemoteRenameFixups_FolderRecordsExcluded(t *testing.T) {
	t.Parallel()
	vaultPath, cleanup := setupRenameTest(t)
	defer cleanup()

	uid := int64(42)
	currentRemote := map[string]model.FileRecord{
		"newfolder/": {Path: "newfolder/", Folder: true, UID: uid, Deleted: false},
		"oldfolder/": {Path: "oldfolder/", Folder: true, UID: uid, Deleted: true},
	}
	previousRemote := map[string]model.FileRecord{}
	previousLocal := map[string]model.FileRecord{}
	currentLocal := map[string]model.FileRecord{}

	logger := zerolog.Nop()
	result := applyRemoteRenameFixups(currentRemote, previousRemote, previousLocal, currentLocal, vaultPath, logger, nil)
	if len(result.Enacted) != 0 {
		t.Fatalf("expected 0 enacted renames (folders excluded), got %d", len(result.Enacted))
	}
}

func TestApplyRemoteRenameFixups_NoUID(t *testing.T) {
	t.Parallel()
	vaultPath, cleanup := setupRenameTest(t)
	defer cleanup()

	// Records with UID=0 should be skipped
	currentRemote := map[string]model.FileRecord{
		"new.md": {Path: "new.md", Hash: "abc", UID: 0, Deleted: false},
		"old.md": {Path: "old.md", Hash: "", UID: 0, Deleted: true},
	}
	previousRemote := map[string]model.FileRecord{
		"old.md": {Path: "old.md", Hash: "abc", UID: 0},
	}
	previousLocal := map[string]model.FileRecord{}
	currentLocal := map[string]model.FileRecord{}

	logger := zerolog.Nop()
	result := applyRemoteRenameFixups(currentRemote, previousRemote, previousLocal, currentLocal, vaultPath, logger, nil)
	if len(result.Enacted) != 0 {
		t.Fatalf("expected 0 enacted renames (UID=0 skipped)")
	}
}

func TestApplyRemoteRenameFixups_SamePath(t *testing.T) {
	t.Parallel()
	vaultPath, cleanup := setupRenameTest(t)
	defer cleanup()

	// Same UID but same path (e.g. file updated, not renamed)
	uid := int64(42)
	currentRemote := map[string]model.FileRecord{
		"file.md": {Path: "file.md", Hash: "abc", UID: uid, Deleted: false},
	}
	// No deleted entry → should be no-op
	logger := zerolog.Nop()
	result := applyRemoteRenameFixups(currentRemote,
		map[string]model.FileRecord{},
		map[string]model.FileRecord{},
		map[string]model.FileRecord{},
		vaultPath, logger, nil,
	)
	if len(result.Enacted) != 0 {
		t.Fatalf("expected no enacted renames")
	}
}

func TestApplyRemoteRenameFixups_DestinationExists(t *testing.T) {
	t.Parallel()
	vaultPath, cleanup := setupRenameTest(t)
	defer cleanup()

	uid := int64(42)
	// Remote renamed: old.md → new.md
	currentRemote := map[string]model.FileRecord{
		"new.md": {Path: "new.md", Hash: "abc", UID: uid, Deleted: false},
		"old.md": {Path: "old.md", Hash: "", UID: uid, Deleted: true},
	}
	previousRemote := map[string]model.FileRecord{
		"old.md": {Path: "old.md", Hash: "abc", UID: uid},
	}
	previousLocal := map[string]model.FileRecord{
		"old.md": {Path: "old.md", Hash: "abc"},
		"new.md": {Path: "new.md", Hash: "existing"},
	}
	// Local already has new.md (destination exists)
	currentLocal := map[string]model.FileRecord{
		"old.md": {Path: "old.md", Hash: "abc"},
		"new.md": {Path: "new.md", Hash: "existing"},
	}

	// Create both files on disk
	localOldPath := filepath.Join(vaultPath, "old.md")
	localNewPath := filepath.Join(vaultPath, "new.md")
	if err := os.WriteFile(localOldPath, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(localNewPath, []byte("existing content"), 0644); err != nil {
		t.Fatal(err)
	}

	logger := zerolog.Nop()
	result := applyRemoteRenameFixups(currentRemote, previousRemote, previousLocal, currentLocal, vaultPath, logger, nil)

	// Should NOT enact rename (destination exists)
	if len(result.Enacted) != 0 {
		t.Fatalf("expected 0 enacted renames (destination exists), got %d", len(result.Enacted))
	}
	if len(result.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(result.Conflicts))
	}
	if result.Conflicts[0] != "old.md" {
		t.Fatalf("expected conflict path 'old.md', got '%s'", result.Conflicts[0])
	}

	// old.md should still exist on disk (not renamed/overwritten)
	if _, err := os.Stat(localOldPath); err != nil {
		t.Error("old.md should still exist on disk")
	}
	// new.md should still have its original content
	content, err := os.ReadFile(localNewPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "existing content" {
		t.Errorf("new.md content was overwritten; got %q, want %q", string(content), "existing content")
	}
	// old.md should still be in currentLocal
	if _, exists := currentLocal["old.md"]; !exists {
		t.Error("old.md should still be in currentLocal")
	}
}

func TestApplyRemoteRenameFixups_RenameError(t *testing.T) {
	// NOTE: Not parallel because we chmod the temp dir
	vaultPath := t.TempDir()

	uid := int64(42)
	currentRemote := map[string]model.FileRecord{
		"new.md": {Path: "new.md", Hash: "abc", UID: uid, Deleted: false},
		"old.md": {Path: "old.md", Hash: "", UID: uid, Deleted: true},
	}
	previousRemote := map[string]model.FileRecord{
		"old.md": {Path: "old.md", Hash: "abc", UID: uid},
	}
	previousLocal := map[string]model.FileRecord{
		"old.md": {Path: "old.md", Hash: "abc"},
	}
	currentLocal := map[string]model.FileRecord{
		"old.md": {Path: "old.md", Hash: "abc"},
	}

	// Create the local file
	localOldPath := filepath.Join(vaultPath, "old.md")
	if err := os.WriteFile(localOldPath, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	// Make vaultPath read-only to prevent os.Rename from creating new.md
	if err := os.Chmod(vaultPath, 0500); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chmod(vaultPath, 0700); err != nil {
			t.Errorf("failed to restore permissions: %v", err)
		}
	}()

	logger := zerolog.Nop()
	result := applyRemoteRenameFixups(currentRemote, previousRemote, previousLocal, currentLocal, vaultPath, logger, nil)
	if len(result.Enacted) != 0 {
		t.Fatalf("expected 0 enacted renames, got %d", len(result.Enacted))
	}
	if len(result.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict on rename failure, got %d", len(result.Conflicts))
	}
	if result.Conflicts[0] != "old.md" {
		t.Fatalf("expected conflict 'old.md', got '%s'", result.Conflicts[0])
	}
	// currentRemote should still have old.md (not deleted, since rename failed)
	if _, exists := currentRemote["old.md"]; !exists {
		t.Error("old.md should remain in currentRemote after rename failure")
	}
}

func TestApplyRemoteRenameFixups_DifferentUIDsSameHash(t *testing.T) {
	t.Parallel()
	vaultPath, cleanup := setupRenameTest(t)
	defer cleanup()

	// Different UIDs but same hash → should detect as rename via hash fallback.
	currentRemote := map[string]model.FileRecord{
		"new.md": {Path: "new.md", Hash: "abc", UID: 99, Deleted: false},
		"old.md": {Path: "old.md", Hash: "abc", UID: 42, Deleted: true},
	}
	previousRemote := map[string]model.FileRecord{
		"old.md": {Path: "old.md", Hash: "abc", UID: 42},
	}
	previousLocal := map[string]model.FileRecord{
		"old.md": {Path: "old.md", Hash: "abc", UID: 42},
	}
	currentLocal := map[string]model.FileRecord{
		"old.md": {Path: "old.md", Hash: "abc"},
	}

	localOldPath := filepath.Join(vaultPath, "old.md")
	if err := os.WriteFile(localOldPath, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	logger := zerolog.Nop()
	result := applyRemoteRenameFixups(currentRemote, previousRemote, previousLocal, currentLocal, vaultPath, logger, nil)

	if len(result.Enacted) != 1 {
		t.Fatalf("expected 1 enacted rename, got %d", len(result.Enacted))
	}
	if result.Enacted[0].OldPath != "old.md" || result.Enacted[0].NewPath != "new.md" {
		t.Fatalf("unexpected rename pair: %+v", result.Enacted[0])
	}
	if len(result.Conflicts) != 0 {
		t.Fatalf("expected 0 conflicts, got %d", len(result.Conflicts))
	}
	if _, exists := currentRemote["old.md"]; exists {
		t.Error("old.md should have been removed from currentRemote")
	}
	if _, exists := currentLocal["old.md"]; exists {
		t.Error("old.md should have been removed from currentLocal")
	}
	if _, exists := currentLocal["new.md"]; !exists {
		t.Error("new.md should be in currentLocal")
	}
	if _, err := os.Stat(filepath.Join(vaultPath, "old.md")); !os.IsNotExist(err) {
		t.Error("old.md should not exist on disk after rename")
	}
	if _, err := os.Stat(filepath.Join(vaultPath, "new.md")); err != nil {
		t.Error("new.md should exist on disk after rename")
	}
}

func TestApplyRemoteRenameFixups_UIDZeroSameHash(t *testing.T) {
	t.Parallel()
	vaultPath, cleanup := setupRenameTest(t)
	defer cleanup()

	// Deleted record with UID=0 (server omitted uid), same hash → should detect via hash fallback.
	currentRemote := map[string]model.FileRecord{
		"new.md": {Path: "new.md", Hash: "abc", UID: 99, Deleted: false},
		"old.md": {Path: "old.md", Hash: "", UID: 0, Deleted: true},
	}
	previousRemote := map[string]model.FileRecord{
		"old.md": {Path: "old.md", Hash: "abc", UID: 0},
	}
	previousLocal := map[string]model.FileRecord{
		"old.md": {Path: "old.md", Hash: "abc", UID: 0},
	}
	currentLocal := map[string]model.FileRecord{
		"old.md": {Path: "old.md", Hash: "abc"},
	}

	localOldPath := filepath.Join(vaultPath, "old.md")
	if err := os.WriteFile(localOldPath, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	logger := zerolog.Nop()
	result := applyRemoteRenameFixups(currentRemote, previousRemote, previousLocal, currentLocal, vaultPath, logger, nil)

	if len(result.Enacted) != 1 {
		t.Fatalf("expected 1 enacted rename, got %d", len(result.Enacted))
	}
	if result.Enacted[0].OldPath != "old.md" || result.Enacted[0].NewPath != "new.md" {
		t.Fatalf("unexpected rename pair: %+v", result.Enacted[0])
	}
	if len(result.Conflicts) != 0 {
		t.Fatalf("expected 0 conflicts, got %d", len(result.Conflicts))
	}
	if _, exists := currentRemote["old.md"]; exists {
		t.Error("old.md should have been removed from currentRemote")
	}
}

func TestApplyRemoteRenameFixups_DifferentUIDsDifferentHashes(t *testing.T) {
	t.Parallel()
	vaultPath, cleanup := setupRenameTest(t)
	defer cleanup()

	// Different UIDs and different hashes → should NOT detect as rename.
	currentRemote := map[string]model.FileRecord{
		"new.md": {Path: "new.md", Hash: "def", UID: 99, Deleted: false},
		"old.md": {Path: "old.md", Hash: "abc", UID: 42, Deleted: true},
	}
	previousRemote := map[string]model.FileRecord{
		"old.md": {Path: "old.md", Hash: "abc", UID: 42},
	}
	previousLocal := map[string]model.FileRecord{
		"old.md": {Path: "old.md", Hash: "abc", UID: 42},
	}
	currentLocal := map[string]model.FileRecord{
		"old.md": {Path: "old.md", Hash: "abc"},
	}

	logger := zerolog.Nop()
	result := applyRemoteRenameFixups(currentRemote, previousRemote, previousLocal, currentLocal, vaultPath, logger, nil)

	if len(result.Enacted) != 0 {
		t.Fatalf("expected 0 enacted renames, got %d", len(result.Enacted))
	}
	if len(result.Conflicts) != 0 {
		t.Fatalf("expected 0 conflicts, got %d", len(result.Conflicts))
	}
	if _, exists := currentRemote["old.md"]; !exists {
		t.Error("old.md should still be in currentRemote (different hashes, not a rename)")
	}
}

func TestApplyRemoteRenameFixups_MultipleHashMatches(t *testing.T) {
	t.Parallel()
	vaultPath, cleanup := setupRenameTest(t)
	defer cleanup()

	// Multiple renames with matching hashes → should detect all.
	currentRemote := map[string]model.FileRecord{
		"new1.md": {Path: "new1.md", Hash: "abc", UID: 101, Deleted: false},
		"old1.md": {Path: "old1.md", Hash: "abc", UID: 1, Deleted: true},
		"new2.md": {Path: "new2.md", Hash: "def", UID: 102, Deleted: false},
		"old2.md": {Path: "old2.md", Hash: "def", UID: 2, Deleted: true},
	}
	previousRemote := map[string]model.FileRecord{
		"old1.md": {Path: "old1.md", Hash: "abc", UID: 1},
		"old2.md": {Path: "old2.md", Hash: "def", UID: 2},
	}
	previousLocal := map[string]model.FileRecord{
		"old1.md": {Path: "old1.md", Hash: "abc", UID: 1},
		"old2.md": {Path: "old2.md", Hash: "def", UID: 2},
	}
	currentLocal := map[string]model.FileRecord{
		"old1.md": {Path: "old1.md", Hash: "abc"},
		"old2.md": {Path: "old2.md", Hash: "def"},
	}

	if err := os.WriteFile(filepath.Join(vaultPath, "old1.md"), []byte("hello1"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vaultPath, "old2.md"), []byte("hello2"), 0644); err != nil {
		t.Fatal(err)
	}

	logger := zerolog.Nop()
	result := applyRemoteRenameFixups(currentRemote, previousRemote, previousLocal, currentLocal, vaultPath, logger, nil)

	if len(result.Enacted) != 2 {
		t.Fatalf("expected 2 enacted renames, got %d", len(result.Enacted))
	}
	if len(result.Conflicts) != 0 {
		t.Fatalf("expected 0 conflicts, got %d", len(result.Conflicts))
	}

	enactedPaths := make(map[string]string)
	for _, pair := range result.Enacted {
		enactedPaths[pair.OldPath] = pair.NewPath
	}
	if enactedPaths["old1.md"] != "new1.md" {
		t.Errorf("expected old1.md -> new1.md, got %s", enactedPaths["old1.md"])
	}
	if enactedPaths["old2.md"] != "new2.md" {
		t.Errorf("expected old2.md -> new2.md, got %s", enactedPaths["old2.md"])
	}

	if _, exists := currentRemote["old1.md"]; exists {
		t.Error("old1.md should have been removed from currentRemote")
	}
	if _, exists := currentRemote["old2.md"]; exists {
		t.Error("old2.md should have been removed from currentRemote")
	}
	if _, exists := currentLocal["new1.md"]; !exists {
		t.Error("new1.md should be in currentLocal")
	}
	if _, exists := currentLocal["new2.md"]; !exists {
		t.Error("new2.md should be in currentLocal")
	}
}

func TestApplyRemoteRenameFixups_AmbiguousHashMatch(t *testing.T) {
	t.Parallel()
	vaultPath, cleanup := setupRenameTest(t)
	defer cleanup()

	// Two DIFFERENT deleted files both have hash "shared",
	// two DIFFERENT new files both have hash "shared".
	// This creates an ambiguous hash match → skip.
	currentRemote := map[string]model.FileRecord{
		"new1.md": {Path: "new1.md", Hash: "shared", UID: 101, Deleted: false},
		"new2.md": {Path: "new2.md", Hash: "shared", UID: 102, Deleted: false},
		"old1.md": {Path: "old1.md", Hash: "shared", UID: 1, Deleted: true},
		"old2.md": {Path: "old2.md", Hash: "shared", UID: 2, Deleted: true},
	}
	previousRemote := map[string]model.FileRecord{
		"old1.md": {Path: "old1.md", Hash: "shared", UID: 1},
		"old2.md": {Path: "old2.md", Hash: "shared", UID: 2},
	}
	previousLocal := map[string]model.FileRecord{}
	currentLocal := map[string]model.FileRecord{}

	logger := zerolog.Nop()
	result := applyRemoteRenameFixups(currentRemote, previousRemote, previousLocal, currentLocal, vaultPath, logger, nil)

	if len(result.Enacted) != 0 {
		t.Fatalf("expected 0 enacted renames (ambiguous hash match), got %d", len(result.Enacted))
	}
	if len(result.Conflicts) != 0 {
		t.Fatalf("expected 0 conflicts, got %d", len(result.Conflicts))
	}
	// Both old entries should remain in currentRemote (not deleted, since renaming was skipped)
	if _, exists := currentRemote["old1.md"]; !exists {
		t.Error("old1.md should still be in currentRemote (ambiguous match, skipped)")
	}
	if _, exists := currentRemote["old2.md"]; !exists {
		t.Error("old2.md should still be in currentRemote (ambiguous match, skipped)")
	}
}

func TestApplyRemoteRenameFixups_EmptyHashNoFallback(t *testing.T) {
	t.Parallel()
	vaultPath, cleanup := setupRenameTest(t)
	defer cleanup()

	// Deleted record has empty hash and no previousRemote fallback available.
	currentRemote := map[string]model.FileRecord{
		"new.md": {Path: "new.md", Hash: "abc", UID: 99, Deleted: false},
		"old.md": {Path: "old.md", Hash: "", UID: 42, Deleted: true},
	}
	previousRemote := map[string]model.FileRecord{}
	previousLocal := map[string]model.FileRecord{}
	currentLocal := map[string]model.FileRecord{}

	logger := zerolog.Nop()
	result := applyRemoteRenameFixups(currentRemote, previousRemote, previousLocal, currentLocal, vaultPath, logger, nil)

	if len(result.Enacted) != 0 {
		t.Fatalf("expected 0 enacted renames (empty hash, no fallback), got %d", len(result.Enacted))
	}
	if len(result.Conflicts) != 0 {
		t.Fatalf("expected 0 conflicts, got %d", len(result.Conflicts))
	}
	// old.md should remain in currentRemote
	if _, exists := currentRemote["old.md"]; !exists {
		t.Error("old.md should still be in currentRemote (empty hash, no fallback)")
	}
}
