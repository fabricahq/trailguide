// Exercise native CLI composition against original library fixtures and real Git history.

package acceptance

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestNativeLifecycle proves authoring, sync, offline operation, and failure preservation through actual processes.
func TestNativeLifecycle(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "trailguide")
	command := exec.Command("go", "build", "-o", binary, "../../../cmd/trailguide")
	if data, err := command.CombinedOutput(); err != nil {
		t.Fatal(err, string(data))
	}
	t.Setenv("TRAILGUIDE_ACCEPTANCE_POISON", "must not reach CLI")
	wrapper := filepath.Join(t.TempDir(), "isolated-cli")
	script := fmt.Sprintf("#!/bin/sh\nif [ -n \"$TRAILGUIDE_ACCEPTANCE_POISON\" ]; then echo 'application environment leaked' >&2; exit 99; fi\nexec '%s' \"$@\"\n", binary)
	if err := os.WriteFile(wrapper, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"lifecycle", "stale-output", "versions", "changed-vendor", "failed-sync"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			started := time.Now()
			report, err := Run(ctx, wrapper, scenario)
			// The duration shows how close a scenario came to its deadline under the machine's load.
			t.Logf("scenario %s took %s in %d steps", scenario, time.Since(started).Round(time.Millisecond), len(report.Steps))
			if err != nil {
				t.Fatalf("%v\n%+v", err, report.Steps)
			}
			if len(report.Verified) < 6 || len(report.Files) == 0 {
				t.Fatal("pilot returned incomplete evidence", report)
			}
		})
	}
}

// TestPilotRejectsWritingChecks proves that extra files and empty directories cannot falsely pass read-only checks.
func TestPilotRejectsWritingChecks(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "trailguide")
	if data, err := exec.Command("go", "build", "-o", binary, "../../../cmd/trailguide").CombinedOutput(); err != nil {
		t.Fatal(err, string(data))
	}
	for _, change := range []string{"printf unexpected > unrelated-user-file.txt", "/bin/mkdir unexpected-empty-directory"} {
		t.Run(change, func(t *testing.T) {
			wrapper := filepath.Join(t.TempDir(), "wrapper")
			script := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = project ] && [ \"$2\" = check ]; then\n IFS= read -r line < .trailguide/_managed/generated/RULES.md\n if [ \"$line\" = 'Stale output' ]; then %s; fi\nfi\nexec '%s' \"$@\"\n", change, binary)
			if err := os.WriteFile(wrapper, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			report, err := Run(ctx, wrapper, "stale-output")
			if err == nil || !strings.Contains(err.Error(), "read-only check changed project") {
				t.Fatal("mutation not detected", err)
			}
			if string(report.Files[".trailguide/_managed/generated/RULES.md"]) != "Stale output" {
				t.Fatal("failed pilot omitted observed project files", report.Files)
			}
		})
	}
}

// TestPilotRejectsSilentRefusal ensures a failed command cannot pass without a consumer-visible diagnostic.
func TestPilotRejectsSilentRefusal(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "trailguide")
	if data, err := exec.Command("go", "build", "-o", binary, "../../../cmd/trailguide").CombinedOutput(); err != nil {
		t.Fatal(err, string(data))
	}
	wrapper := filepath.Join(t.TempDir(), "wrapper")
	script := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = project ] && [ \"$2\" = build ] && [ -f .trailguide/_managed/vendor/team/LICENSE.md ]; then\n IFS= read -r line < .trailguide/_managed/vendor/team/LICENSE.md\n if [ \"$line\" = 'Manual edit' ]; then exit 1; fi\nfi\nexec '%s' \"$@\"\n", binary)
	if err := os.WriteFile(wrapper, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if _, err := Run(ctx, wrapper, "changed-vendor"); err == nil || !strings.Contains(err.Error(), "refusal returned no diagnostic") {
		t.Fatal("silent refusal passed", err)
	}
}

// TestFailedPilotCapture preserves the command failure and observations, including after cancellation.
func TestFailedPilotCapture(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "trailguide")
	if data, err := exec.Command("go", "build", "-o", binary, "../../../cmd/trailguide").CombinedOutput(); err != nil {
		t.Fatal(err, string(data))
	}
	for _, scenario := range []string{"failure", "capture failure", "cancellation"} {
		t.Run(scenario, func(t *testing.T) {
			directory := t.TempDir()
			wrapper := filepath.Join(directory, "wrapper")
			marker := filepath.Join(directory, "ready")
			action := "echo 'deliberate CLI failure' >&2; exit 37"
			if scenario == "capture failure" {
				action = "/bin/ln -s observation.txt unsafe-link; " + action
			}
			if scenario == "cancellation" {
				action = fmt.Sprintf("printf ready > '%s'; exec /bin/sleep 30", marker)
			}
			script := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = project ] && [ \"$2\" = init ]; then\n printf 'observed before failure' > observation.txt\n %s\nfi\nexec '%s' \"$@\"\n", action, binary)
			if err := os.WriteFile(wrapper, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if scenario == "cancellation" {
				go func() {
					ticker := time.NewTicker(10 * time.Millisecond)
					defer ticker.Stop()
					for {
						select {
						case <-ctx.Done():
							return
						case <-ticker.C:
							if _, err := os.Stat(marker); err == nil {
								cancel()
								return
							}
						}
					}
				}()
			}
			report, err := Run(ctx, wrapper, "lifecycle")
			if err == nil {
				t.Fatal("failed pilot reported success")
			}
			if scenario != "cancellation" && !strings.Contains(err.Error(), "deliberate CLI failure") {
				t.Fatal("original command error was lost", err)
			}
			if scenario == "capture failure" {
				if !strings.Contains(err.Error(), "capture acceptance project files") {
					t.Fatal("capture failure was hidden", err)
				}
			} else if string(report.Files["observation.txt"]) != "observed before failure" {
				t.Fatal("failed pilot omitted observation", err, report.Files)
			}
		})
	}
}
