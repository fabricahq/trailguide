// Exercise that the compiled CLI never shows text from Git, a Git server, or the GitHub CLI, in human or JSON
// output, and explains each failure with its own static message instead.

package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/test/ghfixture"
	"github.com/fabricahq/trailguide/internal/test/gitfixture"
)

// marker appears in every probe's external text, so finding any of it in output shows that text passed through.
const marker = "EXTERNAL-TEXT-MARKER"

// noExternalText is how a refused push says where to read the messages Trail Guide doesn't show.
const noExternalText = "Trail Guide doesn't show messages from Git or the server; to read them, push a test tag with git push."

// failureOutput runs args in dir with environment, in human and JSON output, and returns the human stderr, the JSON
// stdout, and the JSON error, requiring exit code 1 and the JSON error code.
func failureOutput(t *testing.T, binary, dir string, environment []string, code string, args ...string) (string, string, responseError) {
	t.Helper()
	out, human, exit := runCLIWithEnvironment(t, binary, dir, environment, args...)
	if exit != 1 || out != "" {
		t.Fatalf("exit %d, stdout %q, stderr:\n%s", exit, out, human)
	}
	structured, diagnostic, exit := runCLIWithEnvironment(t, binary, dir, environment, append(args, "--json")...)
	var response struct {
		OK    bool
		Error responseError
	}
	if err := json.Unmarshal([]byte(structured), &response); err != nil || exit != 1 || diagnostic != "" || response.OK || response.Error.Code != code {
		t.Fatalf("exit %d, %v, stderr %q, stdout:\n%s; want code %s", exit, err, diagnostic, structured, code)
	}
	return human, structured, response.Error
}

// requireStatic checks that the human stderr and the JSON message each contain every one of explanations, and that
// neither they nor the whole JSON output contain any of external.
func requireStatic(t *testing.T, human, structured string, failure responseError, explanations, external []string) {
	t.Helper()
	for name, text := range map[string]string{"stderr": human, "JSON message": failure.Message} {
		for _, want := range explanations {
			if !strings.Contains(text, want) {
				t.Errorf("%s lacks %q:\n%s", name, want, text)
			}
		}
	}
	for name, text := range map[string]string{"stderr": human, "JSON output": structured} {
		for _, secret := range external {
			if strings.Contains(text, secret) {
				t.Errorf("%s shows %q:\n%s", name, secret, text)
			}
		}
	}
}

// refusingHook installs a pre-receive hook in the fixture's repository that runs script and then refuses the push.
func refusingHook(t *testing.T, fixture *gitfixture.Fixture, script string) {
	t.Helper()
	hooks := filepath.Join(fixture.Worktree(), ".git", "hooks")
	if err := os.MkdirAll(hooks, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hooks, "pre-receive"), []byte("#!/bin/sh\n"+script+"exit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
}

// TestLibraryRelease_ShowsNoTextFromTheServerThatRefusedTheTag pushes, with a token in GH_TOKEN, to a remote whose
// URL may carry a password, and a pre-receive hook that prints text marked with marker and refuses the tag. No byte
// of the hook's text appears, whatever it holds, and the static explanation names the cause.
func TestLibraryRelease_ShowsNoTextFromTheServerThatRefusedTheTag(t *testing.T) {
	binary := buildCLI(t)
	hookDeclined := "Git couldn't push release/2 to origin: a hook on the server declined the tag."
	for _, test := range []struct {
		name, remote, hook string
		explanations       []string
		external           []string
	}{
		{"plain text", "", "echo '" + marker + " Release tags need approval from the maintainers.' >&2\n",
			[]string{hookDeclined}, []string{"approval"}},
		{"token echoed plainly", "", "echo '" + marker + " token opaque-secret-value' >&2\n",
			[]string{hookDeclined}, []string{"opaque", "secret"}},
		{"terminal title sequence carrying the token", "", "printf '" + marker + " \\033]0;opaque-secret-value\\007 done\\n' >&2\n",
			[]string{hookDeclined}, []string{"opaque", "secret", "\x1b", "\\u001b"}},
		{"punctuation-only password", "ssh://fixture-user:%21%40%23%24%25%5E%26%2A@fixture.invalid/rules", "printf '%s\\n' '" + marker + " password !@#$%^&*' >&2\n",
			[]string{hookDeclined}, []string{"!@#$%^&*", "%21%40", "fixture-user"}},
		{"credential a credential helper supplied, echoed by the server", "", "echo '" + marker + " authenticated with helper-credential-7731' >&2\n",
			[]string{hookDeclined}, []string{"helper-credential"}},
		{"credential URL wrapped across three lines", "", "echo '" + marker + " https://someone:abcdefgh' >&2\necho 'ijklmnop' >&2\necho 'qrstuvwx@mirror.invalid/rules' >&2\n",
			[]string{hookDeclined}, []string{"abcdefgh", "ijklmnop", "qrstuvwx", "mirror.invalid"}},
		{"GitHub rule violation", "", "echo 'error: GH013: Repository rule violations found for refs/tags/release/2. " + marker + "' >&2\necho '- Cannot create ref due to creations being restricted.' >&2\n",
			[]string{"Git couldn't push release/2 to origin: a repository rule refused the tag (GitHub error GH013).", "Check the repository's rulesets and tag protection rules"}, []string{"Repository rule violations", "creations being restricted"}},
		{"GitHub code Trail Guide doesn't map to a cause", "", "echo 'error: GH001: Large files detected. " + marker + "' >&2\n",
			[]string{"Git couldn't push release/2 to origin: the server refused the tag for a reason Trail Guide doesn't recognize (GitHub error GH001)."}, []string{"Large files", "rulesets"}},
		{"protected tag", "", "echo '" + marker + ": You are not allowed to create protected tags on this project.' >&2\n",
			[]string{"Git couldn't push release/2 to origin: a repository rule or tag protection refused the tag."}, []string{"not allowed", "GH0"}},
		{"permission denied", "", "echo 'ERROR: Permission to acme/rules.git denied to " + marker + ".' >&2\n",
			[]string{"Git couldn't push release/2 to origin: the server denied access, or authentication failed."}, []string{"acme/rules.git"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture, dir := releasedLibrary(t)
			remote := test.remote
			if remote == "" {
				remote = "ssh://fixture-user:fixture-secret%2Fpass@fixture.invalid/rules"
			}
			if _, err := fixture.CommandIn(context.Background(), dir, "remote", "set-url", "origin", remote); err != nil {
				t.Fatal(err)
			}
			pendingPatch(t, fixture, dir)
			refusingHook(t, fixture, test.hook)
			environment, _ := releaseEnvironment(t, fixture, nil)
			human, structured, failure := failureOutput(t, binary, dir, append(environment, "GH_TOKEN=opaque-secret-value"), "push-failed", "library", "release")
			external := append([]string{marker, "remote:", "remote rejected", "hook declined", "fixture-secret", "fixture.invalid"}, test.external...)
			requireStatic(t, human, structured, failure, append([]string{noExternalText}, test.explanations...), external)
		})
	}
}

// TestLibraryRelease_NeverNamesTheDefaultBranchTheRemoteAdvertises, which reaches the refusal through Git's
// standard output rather than its diagnostics.
func TestLibraryRelease_NeverNamesTheDefaultBranchTheRemoteAdvertises(t *testing.T) {
	binary := buildCLI(t)
	fixture, dir := releasedLibrary(t)
	ctx := context.Background()
	if _, err := fixture.Command(ctx, "branch", marker); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.Command(ctx, "symbolic-ref", "HEAD", "refs/heads/"+marker); err != nil {
		t.Fatal(err)
	}
	environment, _ := releaseEnvironment(t, fixture, nil)
	human, structured, failure := failureOutput(t, binary, dir, environment, "not-default-branch", "library", "release")
	requireStatic(t, human, structured, failure, []string{"library releases are published from origin's default branch, but main tracks main, which isn't it."}, []string{marker})
}

// TestLibraryRelease_HidesTheCredentialsOfASchemeRelativeAddress, which url.Parse reads as an authority without a
// scheme, in the push-destination refusal.
func TestLibraryRelease_HidesTheCredentialsOfASchemeRelativeAddress(t *testing.T) {
	binary := buildCLI(t)
	fixture, dir := releasedLibrary(t)
	ctx := context.Background()
	if _, err := fixture.CommandIn(ctx, dir, "remote", "set-url", "origin", "//user:URL-SECRET@host.invalid/rules"); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.CommandIn(ctx, dir, "remote", "set-url", "--push", "origin", "ssh://other.invalid/rules"); err != nil {
		t.Fatal(err)
	}
	environment, _ := releaseEnvironment(t, fixture, nil)
	human, structured, failure := failureOutput(t, binary, dir, environment, "push-destination", "library", "release")
	requireStatic(t, human, structured, failure, []string{"origin fetches from (a URL that can't be parsed, hidden in case it contains credentials) but pushes to ssh://other.invalid/rules."}, []string{"URL-SECRET", "user:"})
}

// gitHubLibrary returns a library clone whose origin is acme/rules on GitHub.com, with a pending patch.
func gitHubLibrary(t *testing.T) (*gitfixture.Fixture, string) {
	t.Helper()
	fixture, dir := releasedLibrary(t)
	pendingPatch(t, fixture, dir)
	if _, err := fixture.CommandIn(context.Background(), dir, "remote", "set-url", "origin", "git@github.com:acme/rules.git"); err != nil {
		t.Fatal(err)
	}
	return fixture, dir
}

// gh calls that publishing release/2 to acme/rules makes.
var (
	ghAuth   = []string{"auth", "status", "--hostname", "github.com"}
	ghView   = []string{"release", "view", "release/2", "--repo", "github.com/acme/rules", "--json", "url", "--jq", ".url"}
	ghCreate = []string{"release", "create", "release/2", "--repo", "github.com/acme/rules", "--verify-tag", "--title", "release/2", "--notes-file", "-"}
)

// TestLibraryRelease_ShowsNoTextFromTheGitHubCLI when looking up or creating the GitHub Release page fails: gh's
// diagnostics, marked with marker, never appear, and the static explanation names the cause and a next step that
// fits the failed operation.
func TestLibraryRelease_ShowsNoTextFromTheGitHubCLI(t *testing.T) {
	binary := buildCLI(t)
	createFinish := ", then run trailguide library release again to create the page."
	for _, test := range []struct {
		name   string
		lookup bool
		stderr string
		want   string
	}{
		{"rate limited", false, "HTTP 403: API rate limit exceeded for " + marker + "\n", "create the GitHub Release page for release/2: GitHub's API rate limit was reached. The tag is published; wait for the limit to reset" + createFinish},
		{"signed out", false, "HTTP 401: Bad credentials " + marker + "\n", "create the GitHub Release page for release/2: gh isn't signed in to GitHub.com, or GitHub refused its credentials. The tag is published; sign in with gh auth login" + createFinish},
		{"permission denied", false, "HTTP 403: Resource not accessible by integration " + marker + "\n", "create the GitHub Release page for release/2: GitHub denied permission to create releases in acme/rules. The tag is published; check that your GitHub account can create releases there" + createFinish},
		{"not found", false, "HTTP 404: Not Found " + marker + "\n", "create the GitHub Release page for release/2: GitHub couldn't find acme/rules, or your account can't see it. The tag is published; check the repository and your access to it" + createFinish},
		{"other creation failure", false, "token opaque-secret-value rejected " + marker + "\n", "create the GitHub Release page for release/2: gh failed for a reason Trail Guide doesn't recognize. The tag is published; check that gh works with gh auth status, and that it can reach the repository with gh repo view github.com/acme/rules, then run trailguide library release again, which creates the page with its notes only if it's still missing."},
		{"permission denied looking up", true, "HTTP 403: Resource not accessible by integration " + marker + "\n", "look up the GitHub Release page for release/2: GitHub denied permission to read releases in acme/rules. The tag is published; check that your GitHub account can read releases there, then run trailguide library release again to finish."},
		{"other lookup failure", true, "HTTP 502: Bad Gateway " + marker + "\n", "look up the GitHub Release page for release/2: gh failed for a reason Trail Guide doesn't recognize. The tag is published; run gh release view release/2 --repo github.com/acme/rules to read gh's message, then run trailguide library release again to finish."},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture, dir := gitHubLibrary(t)
			responses := []ghfixture.Response{{Args: ghAuth}, {Args: ghView, Stderr: "release not found\n", ExitCode: 1}, {Args: ghCreate, Stderr: test.stderr, ExitCode: 1}}
			if test.lookup {
				responses = []ghfixture.Response{{Args: ghAuth}, {Args: ghView, Stderr: test.stderr, ExitCode: 1}}
			}
			environment, _ := releaseEnvironment(t, fixture, responses)
			human, structured, failure := failureOutput(t, binary, dir, append(environment, "GH_TOKEN=opaque-secret-value"), "github-release-failed", "library", "release")
			requireStatic(t, human, structured, failure, []string{"the GitHub CLI couldn't " + test.want}, []string{marker, "HTTP", "opaque", "Bad credentials", "integration", "Gateway"})
		})
	}
}

// TestLibraryRelease_ShowsTheGitHubReleasePageURLItBuilds from the repository and tag, never the URL gh prints, which
// can carry anything.
func TestLibraryRelease_ShowsTheGitHubReleasePageURLItBuilds(t *testing.T) {
	binary := buildCLI(t)
	page := "https://github.com/acme/rules/releases/tag/release/2"
	printed := page + "?token=ghp_syntheticcredential12345678#" + marker + "\n"
	for _, test := range []struct {
		name      string
		responses []ghfixture.Response
		created   bool
	}{
		{"created", []ghfixture.Response{{Args: ghAuth}, {Args: ghView, Stderr: "release not found\n", ExitCode: 1}, {Args: ghCreate, Stdout: printed}}, true},
		{"found", []ghfixture.Response{{Args: ghAuth}, {Args: ghView, Stdout: printed}}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture, dir := gitHubLibrary(t)
			environment, _ := releaseEnvironment(t, fixture, test.responses)
			out, diagnostic, code := runCLIWithEnvironment(t, binary, dir, environment, "library", "release", "--json")
			var response struct {
				OK    bool
				Value struct{ GitHubRelease json.RawMessage }
			}
			if err := json.Unmarshal([]byte(out), &response); err != nil || code != 0 || diagnostic != "" || !response.OK {
				t.Fatal(err, code, out, diagnostic)
			}
			if got, want := compactJSON(t, response.Value.GitHubRelease), `{"created":`+map[bool]string{true: "true", false: "false"}[test.created]+`,"url":"`+page+`"}`; got != want {
				t.Fatalf("githubRelease %s, want %s", got, want)
			}
			if strings.Contains(out, "token") || strings.Contains(out, marker) {
				t.Fatalf("output shows gh's URL:\n%s", out)
			}
		})
	}
}

// syncFailure initializes a project in a new directory whose only source imports repository, syncs it with
// environment, and returns the failure's human stderr, JSON output, and JSON error, requiring code.
func syncFailure(t *testing.T, binary, repository string, environment []string, code string) (string, string, responseError) {
	t.Helper()
	dir := t.TempDir()
	if out, diagnostic, exit := runCLIWithEnvironment(t, binary, dir, environment, "project", "init"); exit != 0 {
		t.Fatal(exit, out, diagnostic)
	}
	config := "schemaVersion: 2\nsources:\n  team:\n    repository: " + repository + "\n    ref: release/1\n    rules:\n      include:\n        - techs/go/*\n"
	if err := os.WriteFile(filepath.Join(dir, ".trailguide", "config.yaml"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	return failureOutput(t, binary, dir, environment, code, "project", "sync")
}

// sshShim returns an SSH command, for GIT_SSH_COMMAND, that prints text to its diagnostics and fails.
func sshShim(t *testing.T, text string) string {
	t.Helper()
	shim := filepath.Join(t.TempDir(), "ssh")
	if err := os.WriteFile(shim, []byte("#!/bin/sh\nprintf '%s\\n' '"+text+"' >&2\nexit 255\n"), 0700); err != nil {
		t.Fatal(err)
	}
	return shim
}

// TestSync_ExplainsImportFailuresWithoutGitsText classifies what Git and the server print, never shown, into a
// static explanation: a connection failure stays one even when a token as short as on appears in Git's text, and
// a remote that url.*.insteadOf rewrites to carry a credential shows no part of it.
func TestSync_ExplainsImportFailuresWithoutGitsText(t *testing.T) {
	binary := buildCLI(t)
	connection := "Could not connect to the library's repository. Check the repository address and your network connection."
	t.Run("refused connection with a two-letter token", func(t *testing.T) {
		environment := append(isolatedEnvironment(t), "GH_TOKEN=on")
		human, structured, failure := syncFailure(t, binary, "https://127.0.0.1:1/acme/rules.git", environment, "connection-failed")
		requireStatic(t, human, structured, failure, []string{connection}, []string{"refused", "withheld", "port 1"})
	})
	for _, test := range []struct{ name, shim, code, explanation string }{
		{"unresolvable host", "ssh: Could not resolve hostname fixture.invalid: " + marker + " opaque-url-secret", "connection-failed", connection},
		{"missing repository", "ERROR: Repository not found. " + marker + " opaque-url-secret", "not-found-or-no-access", "Could not find the library's repository, or your Git credentials can't read it. Check the repository address and your Git credentials."},
		{"unknown host key", "Host key verification failed. " + marker + " opaque-url-secret", "ssh-host-key-failed", "The library's SSH host key couldn't be verified."},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			global := filepath.Join(home, "gitconfig")
			if err := os.WriteFile(global, []byte("[url \"ssh://reader:opaque-url-secret@fixture.invalid/\"]\n\tinsteadOf = https://rewritten.invalid/\n"), 0600); err != nil {
				t.Fatal(err)
			}
			environment := append(isolatedEnvironment(t), "GIT_CONFIG_GLOBAL="+global, "GIT_SSH_COMMAND="+sshShim(t, test.shim), "GIT_SSH_VARIANT=ssh")
			human, structured, failure := syncFailure(t, binary, "https://rewritten.invalid/acme/rules.git", environment, test.code)
			requireStatic(t, human, structured, failure, []string{test.explanation}, []string{marker, "url-secret", "fixture.invalid", "ERROR", "ssh:"})
		})
	}
}

// isolatedEnvironment returns the process environment without Git's or the user's configuration.
func isolatedEnvironment(t *testing.T) []string {
	t.Helper()
	environment := []string{}
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if !strings.HasPrefix(key, "GIT_") && key != "HOME" && key != "XDG_CONFIG_HOME" && key != "GH_TOKEN" && key != "GITHUB_TOKEN" {
			environment = append(environment, item)
		}
	}
	home := t.TempDir()
	return append(environment, "HOME="+home, "XDG_CONFIG_HOME="+home, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0")
}
