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
	result, err := applyRemoteRenameFixups(currentRemote, previousRemote, previousLocal, currentLocal, vaultPath, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

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
	result, err := applyRemoteRenameFixups(currentRemote, previousRemote, previousLocal, currentLocal, vaultPath, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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
	result, err := applyRemoteRenameFixups(
		map[string]model.FileRecord{},
		map[string]model.FileRecord{},
		map[string]model.FileRecord{},
		map[string]model.FileRecord{},
		vaultPath,
		logger,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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
	result, err := applyRemoteRenameFixups(currentRemote, previousRemote, previousLocal, currentLocal, vaultPath, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

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
	result, err := applyRemoteRenameFixups(currentRemote, previousRemote, previousLocal, currentLocal, vaultPath, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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
	result, err := applyRemoteRenameFixups(currentRemote, previousRemote, previousLocal, currentLocal, vaultPath, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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
	result, err := applyRemoteRenameFixups(currentRemote,
		map[string]model.FileRecord{},
		map[string]model.FileRecord{},
		map[string]model.FileRecord{},
		vaultPath, logger,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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
	result, err := applyRemoteRenameFixups(currentRemote, previousRemote, previousLocal, currentLocal, vaultPath, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

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
	defer os.Chmod(vaultPath, 0700) // restore for cleanup

	logger := zerolog.Nop()
	result, err := applyRemoteRenameFixups(currentRemote, previousRemote, previousLocal, currentLocal, vaultPath, logger)
	if err != nil {
		t.Fatalf("expected nil error on rename failure, got: %v", err)
	}
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
