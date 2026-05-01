package sync

import (
	"testing"
)

func TestThreeWayMerge(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		base    string
		local   string
		remote  string
		want    string
		wantErr bool
	}{
		{
			name:   "non-overlapping edits",
			base:   "A\nB\nC\n",
			local:  "A\nB\nC local\n",
			remote: "A remote\nB\nC\n",
			want:   "A remote\nB\nC local\n",
		},
		{
			name:   "same edit both sides",
			base:   "A\nB\nC\n",
			local:  "A\nB\nD\n",
			remote: "A\nB\nD\n",
			want:   "A\nB\nD\n",
		},
		{
			name:   "local only changed",
			base:   "A\nB\nC\n",
			local:  "A\nB\nD\n",
			remote: "A\nB\nC\n",
			want:   "A\nB\nD\n",
		},
		{
			name:   "remote only changed",
			base:   "A\nB\nC\n",
			local:  "A\nB\nC\n",
			remote: "A\nB\nD\n",
			want:   "A\nB\nD\n",
		},
		{
			// DMP fuzzy matching applies the local patch to remote; remote's Y is silently discarded.
			name:   "conflicting edits",
			base:   "A\nB\nC\n",
			local:  "A\nX\nC\n",
			remote: "A\nY\nC\n",
			want:   "A\nX\nC\n",
		},
		{
			// Remote diverged so far that DMP has no fuzzy anchor; patch application fails.
			name:    "conflict with no patch context match",
			base:    "AAAA\nBBBB\nCCCC\n",
			local:   "AAAA\nXXXX\nCCCC\n",
			remote:  "1111\n2222\n3333\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := threeWayMerge(tt.base, tt.local, tt.remote)
			if (err != nil) != tt.wantErr {
				t.Errorf("threeWayMerge() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("threeWayMerge() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestJSONMerge(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		localJSON  string
		remoteJSON string
		want       string
		wantErr    bool
	}{
		{
			name:       "disjoint keys",
			localJSON:  `{"a": 1}`,
			remoteJSON: `{"b": 2}`,
			want:       "{\n  \"a\": 1,\n  \"b\": 2\n}",
		},
		{
			name:       "server wins conflict",
			localJSON:  `{"a": 1}`,
			remoteJSON: `{"a": 2}`,
			want:       "{\n  \"a\": 2\n}",
		},
		{
			name:       "nested objects",
			localJSON:  `{"x": {"a": 1}}`,
			remoteJSON: `{"x": {"b": 2}}`,
			want:       "{\n  \"x\": {\n    \"a\": 1,\n    \"b\": 2\n  }\n}",
		},
		{
			name:       "invalid local JSON",
			localJSON:  `not json`,
			remoteJSON: `{"a": 1}`,
			wantErr:    true,
		},
		{
			name:       "invalid remote JSON",
			localJSON:  `{"a": 1}`,
			remoteJSON: `not json`,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := jsonMerge(tt.localJSON, tt.remoteJSON)
			if (err != nil) != tt.wantErr {
				t.Errorf("jsonMerge() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("jsonMerge() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsMergeablePath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path string
		want bool
	}{
		{"note.md", true},
		{"dir/note.md", true},
		{"config.json", false},
		{"image.png", false},
		{"noext", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			if got := isMergeablePath(tt.path); got != tt.want {
				t.Errorf("isMergeablePath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestIsJSONConfigPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path      string
		configDir string
		want      bool
	}{
		{".obsidian/app.json", ".obsidian", true},
		{".obsidian/appearance.json", ".obsidian", true},
		{"notes/config.json", ".obsidian", false},
		{".obsidian/theme.css", ".obsidian", false},
		{".obsidian/config.json", "", true},
		{".obsidian/plugins/dataview/data.json", ".obsidian", true},
		{".obsidian/themes/Things/manifest.json", ".obsidian", true},
		{".obsidian\\plugins\\dataview\\data.json", ".obsidian", true},
		{"custom-dir/app.json", "custom-dir", true},
		{"custom-dir/plugins/x/data.json", "custom-dir", true},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			if got := isJSONConfigPath(tt.path, tt.configDir); got != tt.want {
				t.Errorf("isJSONConfigPath(%q, %q) = %v, want %v", tt.path, tt.configDir, got, tt.want)
			}
		})
	}
}
