// Verify revision interpretation at the project input boundary without subprocesses.

package project

import "testing"

// TestParseSourceRefAcceptsOnlyExactRevisions accepts tags, branches, and full commits, and rejects ranges and
// abbreviated commits.
func TestParseSourceRefAcceptsOnlyExactRevisions(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"0123456789012345678901234567890123456789", "0123456789012345678901234567890123456789"},
		{"v1.2.3", "v1.2.3"},
		{"  v1.2.3  ", "v1.2.3"},
		{"1.2.3", "1.2.3"},
		{"release/5", "release/5"},
		{"refs/tags/=special", "refs/tags/=special"},
		{">= 1.2.0, < 2.0.0", ""},
		{"~> 1.2.0", ""},
		{"^1.2.0", ""},
		{"refs/heads/main", "refs/heads/main"},
		{"refs/remotes/origin/main", ""},
		{"abcdef1", ""},
		{"", ""},
	} {
		t.Run(tc.input, func(t *testing.T) {
			ref, err := ParseSourceRef(tc.input)
			if tc.want == "" {
				if err == nil || !ref.IsZero() {
					t.Fatal(ref, err)
				}
				return
			}
			if err != nil || ref.String() != tc.want {
				t.Fatal(ref, err)
			}
		})
	}
}
