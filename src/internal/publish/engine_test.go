package publish

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Belphemur/obsidian-headless/internal/model"
	"github.com/bmatcuk/doublestar/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptr[T any](v T) *T {
	return &v
}

// --- TestDetectPublishFlag ---

func TestDetectPublishFlag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		content  string
		expected *bool
	}{
		{
			name:     "publish_true",
			content:  "---\npublish: true\n---\n# Hello",
			expected: ptr(true),
		},
		{
			name:     "publish_false",
			content:  "---\npublish: false\n---\n# Hello",
			expected: ptr(false),
		},
		{
			name:     "no_frontmatter",
			content:  "# Hello\nworld",
			expected: nil,
		},
		{
			name:     "frontmatter_without_publish_key",
			content:  "---\ntitle: My Note\n---\n# Hello",
			expected: nil,
		},
		{
			name:     "publish_string_not_bool",
			content:  "---\npublish: \"yes\"\n---\n# Hello",
			expected: nil,
		},
		{
			name:     "malformed_yaml",
			content:  "---\npublish: [\n---\n# Hello",
			expected: nil,
		},
		{
			name:     "frontmatter_with_other_keys_and_publish",
			content:  "---\ntitle: My Note\npublish: true\nauthor: Alice\n---\n# Hello",
			expected: ptr(true),
		},
		{
			name:     "windows_line_endings",
			content:  "---\r\npublish: true\r\n---\r\n# Hello",
			expected: ptr(true),
		},
		{
			name:     "publish_not_at_start",
			content:  "# Hello\n---\npublish: true\n---\nmore content",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := detectPublishFlag([]byte(tt.content))
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, *tt.expected, *result)
			}
		})
	}
}

// --- TestMatchesAny ---

func TestMatchesAny(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		path     string
		patterns []string
		expected bool
	}{
		{
			name:     "simple_match",
			path:     "notes/test.md",
			patterns: []string{"notes/*"},
			expected: true,
		},
		{
			name:     "simple_no_match",
			path:     "notes/test.md",
			patterns: []string{"drafts/*"},
			expected: false,
		},
		{
			name:     "wildcard_extension_root_only",
			path:     "test.md",
			patterns: []string{"*.md"},
			expected: true,
		},
		{
			name:     "multiple_patterns_one_matches",
			path:     "notes/test.md",
			patterns: []string{"notes/*.md", "drafts/*"},
			expected: true,
		},
		{
			name:     "nil_patterns",
			path:     "notes/test.md",
			patterns: nil,
			expected: false,
		},
		{
			name:     "doublestar_recursive",
			path:     "notes/deep/test.md",
			patterns: []string{"notes/**"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := matchesAny(tt.path, tt.patterns)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// --- TestReadPublishProbe ---

func TestReadPublishProbe(t *testing.T) {
	t.Parallel()

	t.Run("exactly_probe_size", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "test.md")

		content := make([]byte, publishProbeSize)
		for i := range content {
			content[i] = byte('a' + (i % 26))
		}
		require.NoError(t, os.WriteFile(path, content, 0644))

		probe, err := readPublishProbe(path)
		require.NoError(t, err)
		assert.Len(t, probe, publishProbeSize)
		assert.Equal(t, content, probe)
	})

	t.Run("smaller_than_probe_size", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "test.md")

		content := []byte("---\npublish: true\n---\n# Hello")
		require.NoError(t, os.WriteFile(path, content, 0644))

		probe, err := readPublishProbe(path)
		require.NoError(t, err)
		assert.Equal(t, content, probe)
	})

	t.Run("larger_than_probe_size", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "test.md")

		content := make([]byte, publishProbeSize+1024)
		for i := range content {
			content[i] = byte('a' + (i % 26))
		}
		require.NoError(t, os.WriteFile(path, content, 0644))

		probe, err := readPublishProbe(path)
		require.NoError(t, err)
		assert.Len(t, probe, publishProbeSize)
		assert.Equal(t, content[:publishProbeSize], probe)
	})

	t.Run("non_existent_file", func(t *testing.T) {
		t.Parallel()
		probe, err := readPublishProbe(filepath.Join(t.TempDir(), "nonexistent.md"))
		assert.Error(t, err)
		assert.Nil(t, probe)
	})
}

// --- TestScanLocal ---

func TestScanLocal(t *testing.T) {
	t.Parallel()

	t.Run("basic_inclusion_and_exclusion", func(t *testing.T) {
		t.Parallel()
		vaultDir := t.TempDir()

		files := map[string]string{
			"publish_true.md":       "---\npublish: true\n---\n# Hello",
			"publish_false.md":      "---\npublish: false\n---\n# Hello",
			"no_frontmatter.md":     "# Hello\nworld",
			"drafts/draft.md":       "# Draft",
			".obsidian/config.json": "{}",
			".git/config":           "[core]",
			"notes/nested.md":       "# Nested",
		}

		for relPath, content := range files {
			fullPath := filepath.Join(vaultDir, relPath)
			require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0755))
			require.NoError(t, os.WriteFile(fullPath, []byte(content), 0644))
		}

		// Create a symlink
		symlinkTarget := filepath.Join(vaultDir, "publish_true.md")
		symlinkPath := filepath.Join(vaultDir, "symlink.md")
		require.NoError(t, os.Symlink(symlinkTarget, symlinkPath))

		engine := &Engine{
			Config: model.PublishConfig{
				VaultPath: vaultDir,
				Includes:  []string{"*.md"},
				Excludes:  []string{"drafts/*"},
			},
		}

		localFiles, cache, err := engine.scanLocal(false)
		require.NoError(t, err)

		// publish: true should be included regardless of include/exclude patterns
		assert.Contains(t, localFiles, "publish_true.md")

		// publish: false should be excluded
		assert.NotContains(t, localFiles, "publish_false.md")

		// no_frontmatter.md matches include pattern *.md and doesn't match exclude
		assert.Contains(t, localFiles, "no_frontmatter.md")

		// drafts/draft.md matches exclude pattern
		assert.NotContains(t, localFiles, filepath.ToSlash("drafts/draft.md"))

		// .obsidian and .git files should be excluded
		assert.NotContains(t, localFiles, filepath.ToSlash(".obsidian/config.json"))
		assert.NotContains(t, localFiles, filepath.ToSlash(".git/config"))

		// Symlink should be excluded
		assert.NotContains(t, localFiles, "symlink.md")

		// notes/nested.md doesn't match *.md at root level but it's in notes/
		// With includes=["*.md"], notes/nested.md doesn't match, so it should be excluded when all=false
		assert.NotContains(t, localFiles, filepath.ToSlash("notes/nested.md"))

		// Cache should have same entries as localFiles
		assert.Len(t, cache, len(localFiles))
		for path := range localFiles {
			assert.Contains(t, cache, path)
		}
	})

	t.Run("all_flag_includes_everything", func(t *testing.T) {
		t.Parallel()
		vaultDir := t.TempDir()

		files := map[string]string{
			"notes/test.md":   "# Hello",
			"images/logo.png": "binary",
			"drafts/draft.md": "---\npublish: false\n---\n# Draft",
		}

		for relPath, content := range files {
			fullPath := filepath.Join(vaultDir, relPath)
			require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0755))
			require.NoError(t, os.WriteFile(fullPath, []byte(content), 0644))
		}

		engine := &Engine{
			Config: model.PublishConfig{
				VaultPath: vaultDir,
			},
		}

		localFiles, _, err := engine.scanLocal(true)
		require.NoError(t, err)

		// With all=true, everything is included except publish: false and hidden dirs
		assert.Contains(t, localFiles, filepath.ToSlash("notes/test.md"))
		assert.Contains(t, localFiles, filepath.ToSlash("images/logo.png"))

		// publish: false should still be excluded
		assert.NotContains(t, localFiles, filepath.ToSlash("drafts/draft.md"))
	})

	t.Run("all_false_with_includes", func(t *testing.T) {
		t.Parallel()
		vaultDir := t.TempDir()

		files := map[string]string{
			"notes/test.md":   "# Hello",
			"images/logo.png": "binary",
		}

		for relPath, content := range files {
			fullPath := filepath.Join(vaultDir, relPath)
			require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0755))
			require.NoError(t, os.WriteFile(fullPath, []byte(content), 0644))
		}

		engine := &Engine{
			Config: model.PublishConfig{
				VaultPath: vaultDir,
				Includes:  []string{"*.md", "images/*"},
			},
		}

		localFiles, _, err := engine.scanLocal(false)
		require.NoError(t, err)

		// notes/test.md doesn't match "*.md" (it's in notes/)
		// It also doesn't match "images/*"
		assert.NotContains(t, localFiles, filepath.ToSlash("notes/test.md"))

		// images/logo.png matches "images/*"
		assert.Contains(t, localFiles, filepath.ToSlash("images/logo.png"))
	})

	t.Run("all_false_without_includes_excludes_nothing", func(t *testing.T) {
		t.Parallel()
		vaultDir := t.TempDir()

		files := map[string]string{
			"notes/test.md": "# Hello",
		}

		for relPath, content := range files {
			fullPath := filepath.Join(vaultDir, relPath)
			require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0755))
			require.NoError(t, os.WriteFile(fullPath, []byte(content), 0644))
		}

		engine := &Engine{
			Config: model.PublishConfig{
				VaultPath: vaultDir,
			},
		}

		localFiles, _, err := engine.scanLocal(false)
		require.NoError(t, err)

		// Without includes and all=false, nothing is included
		assert.Empty(t, localFiles)
	})

	t.Run("excludes_override_includes", func(t *testing.T) {
		t.Parallel()
		vaultDir := t.TempDir()

		files := map[string]string{
			"notes/test.md":  "# Hello",
			"notes/draft.md": "# Draft",
		}

		for relPath, content := range files {
			fullPath := filepath.Join(vaultDir, relPath)
			require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0755))
			require.NoError(t, os.WriteFile(fullPath, []byte(content), 0644))
		}

		engine := &Engine{
			Config: model.PublishConfig{
				VaultPath: vaultDir,
				Includes:  []string{"notes/*"},
				Excludes:  []string{"notes/draft.md"},
			},
		}

		localFiles, _, err := engine.scanLocal(false)
		require.NoError(t, err)

		assert.Contains(t, localFiles, filepath.ToSlash("notes/test.md"))
		assert.NotContains(t, localFiles, filepath.ToSlash("notes/draft.md"))
	})

	t.Run("directories_not_included_as_files", func(t *testing.T) {
		t.Parallel()
		vaultDir := t.TempDir()

		require.NoError(t, os.MkdirAll(filepath.Join(vaultDir, "notes"), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(vaultDir, "notes", "test.md"), []byte("# Hello"), 0644))

		engine := &Engine{
			Config: model.PublishConfig{
				VaultPath: vaultDir,
				Includes:  []string{"notes/*"},
			},
		}

		localFiles, _, err := engine.scanLocal(false)
		require.NoError(t, err)

		assert.NotContains(t, localFiles, "notes")
		assert.Contains(t, localFiles, filepath.ToSlash("notes/test.md"))
	})

	t.Run("publish_flag_highest_priority", func(t *testing.T) {
		t.Parallel()
		vaultDir := t.TempDir()

		files := map[string]string{
			"excluded_but_true.md":  "---\npublish: true\n---\n# Hello",
			"included_but_false.md": "---\npublish: false\n---\n# Hello",
		}

		for relPath, content := range files {
			fullPath := filepath.Join(vaultDir, relPath)
			require.NoError(t, os.WriteFile(fullPath, []byte(content), 0644))
		}

		engine := &Engine{
			Config: model.PublishConfig{
				VaultPath: vaultDir,
				Excludes:  []string{"excluded_but_true.md"},
				Includes:  []string{"included_but_false.md"},
			},
		}

		localFiles, _, err := engine.scanLocal(false)
		require.NoError(t, err)

		// publish: true overrides excludes
		assert.Contains(t, localFiles, "excluded_but_true.md")

		// publish: false overrides includes
		assert.NotContains(t, localFiles, "included_but_false.md")
	})
}

// --- TestNewEngine ---

func TestNewEngine(t *testing.T) {
	t.Parallel()

	config := model.PublishConfig{
		SiteID:    "site-123",
		Host:      "https://example.com",
		VaultPath: "/tmp/vault",
		Includes:  []string{"*.md"},
		Excludes:  []string{"drafts/*"},
	}
	token := "test-token"

	// We use a nil client since we can't easily instantiate api.Client,
	// but the struct field assignment is what we're testing.
	engine := NewEngine(nil, config, token)
	require.NotNil(t, engine)
	assert.Nil(t, engine.Client)
	assert.Equal(t, config, engine.Config)
	assert.Equal(t, token, engine.Token)
}

// --- Benchmark / Doublestar sanity check ---

func TestDoublestarSanity(t *testing.T) {
	t.Parallel()

	match, err := doublestar.Match("notes/**", "notes/deep/test.md")
	require.NoError(t, err)
	assert.True(t, match)

	match, err = doublestar.Match("*.md", "notes/test.md")
	require.NoError(t, err)
	assert.False(t, match)
}
