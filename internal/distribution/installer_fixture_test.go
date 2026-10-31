// Provide isolated release archives and download fixtures for the real shell installer.
// Only curl and, except in the native smoke test, uname are replaced.

package distribution

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type installerFixture struct {
	t                                           *testing.T
	root, home, commands, releases, destination string
	binary                                      []byte
	// executable names the release's archive and the executable in it: trailguide, or code-rules for releases made
	// before Code Rules was renamed Trail Guide.
	executable                 string
	platform, arch             string
	shell, zdotdir, configHome string
}

type installerMember struct {
	name    string
	data    []byte
	symlink bool
}

func newInstallerFixture(t *testing.T) *installerFixture {
	t.Helper()
	root := t.TempDir()
	f := &installerFixture{t: t, root: root, home: filepath.Join(root, "home"), commands: filepath.Join(root, "commands"), releases: filepath.Join(root, "releases"), platform: "Linux", arch: "x86_64", shell: "/bin/sh", binary: []byte("#!/bin/sh\nprintf 'trailguide 1.2.3\\n'\n"), executable: "trailguide"}
	f.destination = filepath.Join(f.home, ".local/bin")
	for _, dir := range []string{f.home, f.commands, f.releases} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	f.write(filepath.Join(f.commands, "uname"), []byte("#!/bin/sh\ncase \"$1\" in -s) echo \"$INSTALLER_OS\";; -m) echo \"$INSTALLER_ARCH\";; esac\n"), 0755)
	f.write(filepath.Join(f.commands, "curl"), []byte(installerCurlFixture), 0755)
	f.release("1.2.3", "linux_amd64", nil)
	return f
}

func (f *installerFixture) write(path string, data []byte, mode os.FileMode) {
	f.t.Helper()
	if err := os.WriteFile(path, data, mode); err != nil {
		f.t.Fatal(err)
	}
}

func (f *installerFixture) read(path string) []byte {
	f.t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		f.t.Fatal(err)
	}
	return data
}

// release writes a fresh archive and checksum; nil members select a complete release.
func (f *installerFixture) release(version, target string, members []installerMember) string {
	f.t.Helper()
	if members == nil {
		members = []installerMember{{f.executable, f.binary, false}, {"LICENSE.md", []byte("MIT license"), false}, {"README.txt", []byte("Trail Guide"), false}}
	}
	var data bytes.Buffer
	compressed := gzip.NewWriter(&data)
	archive := tar.NewWriter(compressed)
	for _, member := range members {
		header := &tar.Header{Name: member.name, Mode: 0644, Size: int64(len(member.data)), Typeflag: tar.TypeReg, Format: tar.FormatUSTAR}
		if member.name == f.executable {
			header.Mode = 0755
		}
		if member.symlink {
			header.Typeflag = tar.TypeSymlink
			header.Linkname = "link-target"
			header.Size = 0
		}
		if err := archive.WriteHeader(header); err != nil {
			f.t.Fatal(err)
		}
		if !member.symlink {
			if _, err := archive.Write(member.data); err != nil {
				f.t.Fatal(err)
			}
		}
	}
	if err := archive.Close(); err != nil {
		f.t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		f.t.Fatal(err)
	}
	dir := filepath.Join(f.releases, "v"+version)
	if err := os.MkdirAll(dir, 0755); err != nil {
		f.t.Fatal(err)
	}
	name := fmt.Sprintf("%s_%s_%s.tar.gz", f.executable, version, target)
	path := filepath.Join(dir, name)
	f.write(path, data.Bytes(), 0644)
	f.write(filepath.Join(dir, "SHA256SUMS"), fmt.Appendf(nil, "%x  %s\n", sha256.Sum256(data.Bytes()), name), 0644)
	return path
}

// run executes the installer as piped input, with all writable paths under the fixture root.
func (f *installerFixture) run(args ...string) (string, error) {
	f.t.Helper()
	script := f.read(filepath.Join("..", "..", "docs", "public", "install.sh"))
	ctx, cancel := context.WithTimeout(f.t.Context(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", append([]string{"-s", "--"}, args...)...)
	cmd.Dir = f.root
	cmd.Env = f.environment()
	cmd.Stdin = bytes.NewReader(script)
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		f.t.Fatalf("installer timed out: %s", output)
	}
	return string(output), err
}

func (f *installerFixture) environment() []string {
	return append(os.Environ(), "HOME="+f.home, "SHELL="+f.shell, "ZDOTDIR="+f.zdotdir, "XDG_CONFIG_HOME="+f.configHome, "TMPDIR="+f.root, "FIXTURES="+f.releases, "PATH="+f.commands+string(os.PathListSeparator)+os.Getenv("PATH"), "INSTALLER_OS="+f.platform, "INSTALLER_ARCH="+f.arch)
}

func (f *installerFixture) success(args ...string) string {
	f.t.Helper()
	output, err := f.run(args...)
	if err != nil {
		f.t.Fatalf("installer failed: %v\n%s", err, output)
	}
	return output
}

func (f *installerFixture) failure(message string, args ...string) {
	f.t.Helper()
	output, err := f.run(args...)
	if err == nil || !strings.Contains(output, message) {
		f.t.Fatalf("wanted failure containing %q; got %v\n%s", message, err, output)
	}
}

func (f *installerFixture) content(path string, want []byte) {
	f.t.Helper()
	if got := f.read(path); !bytes.Equal(got, want) {
		f.t.Fatalf("unexpected content at %s: %q", path, got)
	}
}

func (f *installerFixture) absent(path string) {
	f.t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		f.t.Fatalf("expected absent %s, got %v", path, err)
	}
}

// The fixture enforces HTTPS flags and serves only local release bytes, never network traffic.
const installerCurlFixture = `#!/bin/sh
set -eu
proto= redir= tls= output= url=
while [ "$#" -gt 0 ]; do
 case "$1" in
  --proto) proto=$2; shift 2 ;;
  --proto-redir) redir=$2; shift 2 ;;
  --tlsv1.2) tls=yes; shift ;;
  --output) output=$2; shift 2 ;;
  --retry|--write-out) shift 2 ;;
  --fail|--silent|--show-error|--location) shift ;;
  https://*) url=$1; shift ;;
  *) echo 'unexpected curl fixture argument' >&2; exit 2 ;;
 esac
done
[ "$proto" = '=https' ] && [ "$redir" = '=https' ] && [ "$tls" = yes ]
base=https://github.com/fabricahq/trailguide/releases
case "$url" in
 "$base/latest") printf '%s' "$base/tag/v1.2.3" ;;
 "$base/download/"*)
  source="$FIXTURES/${url#"$base/download/"}"
  [ -f "$source" ] || exit 22
  cp "$source" "$output"
  ;;
 *) echo 'unexpected curl fixture URL' >&2; exit 2 ;;
esac
`
