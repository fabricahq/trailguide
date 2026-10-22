// Prove unsupported guide publication fails before live output is displaced or recovery is required.

package filetxn

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

// TestUnsupportedGuideRenamePreservesLiveOutput exercises the complete writer with a failing platform primitive.
func TestUnsupportedGuideRenamePreservesLiveOutput(t *testing.T) {
	for _, unsupported := range []error{unix.EINVAL, unix.ENOTSUP, unix.ENOSYS} {
		for _, guide := range []Target{GuideReadme, GuideStandalone} {
			t.Run(string(guide)+"/"+unsupported.Error(), func(t *testing.T) {
				root := openProject(t)
				writeFixture(t, root, string(guide), "old guide")
				writeFixture(t, root, "_managed/generated/old", "old output")
				writeFixture(t, root, "_managed/vendor/old", "old library")
				calls := 0
				blocked := func(int, string, int, string) error { calls++; return unsupported }
				operation := func(w *Writer) error {
					w.rename = func(from, to string) error { return renameManagedWith(root, from, to, blocked) }
					return w.Apply(map[Target]map[string][]byte{guide: {string(guide): []byte("new guide")}, Generated: {"new": []byte("new output")}, Vendor: {"new": []byte("new library")}}, nil)
				}
				// Retrying on the same filesystem must fail safely, rather than becoming stuck in recovery.
				for range 2 {
					err := WithWriter(context.Background(), root, operation)
					if !errors.Is(err, unsupported) || !strings.Contains(err.Error(), "existing files were not changed") {
						t.Fatal("expected safe publication refusal", err)
					}
					for name, want := range map[string]string{string(guide): "old guide", "_managed/generated/old": "old output", "_managed/vendor/old": "old library"} {
						data, err := root.ReadFile(name)
						if err != nil || string(data) != want {
							t.Fatal(name, string(data), err)
						}
					}
					if err := RequireIdle(root); err != nil {
						t.Fatal("left recovery state", err)
					}
				}
				if calls != 2 {
					t.Fatal("platform primitive was not probed on each attempt", calls)
				}
			})
		}
	}
}

// TestGuideRenameProbeKeepsNoReplaceGuarantee checks that the staging probe uses exclusive publication too.
func TestGuideRenameProbeKeepsNoReplaceGuarantee(t *testing.T) {
	root := openProject(t)
	writeFixture(t, root, "README.md", "old guide")
	err := WithWriter(context.Background(), root, func(w *Writer) error {
		rename := w.rename
		w.rename = func(from, to string) error {
			if strings.Contains(to, "rename-check/") {
				writeFixture(t, root, to, "unexpected staging file")
			}
			return rename(from, to)
		}
		return w.Apply(map[Target]map[string][]byte{GuideReadme: {"README.md": []byte("new guide")}}, nil)
	})
	if !errors.Is(err, os.ErrExist) {
		t.Fatal("probe replaced a file", err)
	}
	data, err := root.ReadFile("README.md")
	if err != nil || string(data) != "old guide" {
		t.Fatal(string(data), err)
	}
	if err := RequireIdle(root); err != nil {
		t.Fatal(err)
	}
}
