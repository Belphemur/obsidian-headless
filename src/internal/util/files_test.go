package util

import "testing"

func TestIsLegalPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path string
		want bool
	}{
		{"note.md", true},
		{"folder/note.md", true},
		{".obsidian/plugins/data.json", true},
		{"", false},
		{"note:colon.md", false},
		{"folder/note:colon.md", false},
		{"note*star.md", false},
		{"note?question.md", false},
		{"note\"quote.md", false},
		{"note<less.md", false},
		{"note>greater.md", false},
		{"note|pipe.md", false},
		{"note\\backslash.md", false},
		{"note\x00null.md", false},
		{"note\x01control.md", false},
		{"../escape.md", false},
		{"./dot.md", false},
		{"folder//double.md", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			got := IsLegalPath(tt.path)
			if got != tt.want {
				t.Errorf("IsLegalPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}
