// Verify that library init records minTrailguideVersion and library commands refuse an older Trail Guide.

package library

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestInitialize_RecordsTheReleaseVersion writes a release build's version into a new configuration, and nothing
// for a development build.
func TestInitialize_RecordsTheReleaseVersion(t *testing.T) {
	for version, want := range map[string]bool{"0.2.0": true, "0.0.0-development": false} {
		t.Run(version, func(t *testing.T) {
			options := Options{Directory: t.TempDir(), ToolVersion: version}
			if _, err := Initialize(context.Background(), options, "", nil); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(options.Directory, ".trailguide", "config.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.HasPrefix(string(data), "schemaVersion: 2\nminTrailguideVersion: 0.2.0\nsources: {}\n"); got != want {
				t.Fatalf("config.yaml:\n%s", data)
			}
		})
	}
}

// TestCheck_RefusesAnOlderTrailguide fails with upgrade-required when the library's configuration requires a newer
// release, even when it has fields this version doesn't know.
func TestCheck_RefusesAnOlderTrailguide(t *testing.T) {
	ctx := context.Background()
	options := Options{Directory: t.TempDir(), ToolVersion: "0.2.0"}
	if _, err := Initialize(ctx, options, "", nil); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(options.Directory, ".trailguide", "config.yaml")
	data, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	newer := strings.Replace(string(data), "minTrailguideVersion: 0.2.0\n", "minTrailguideVersion: 0.3.0\nfutureSetting: true\n", 1)
	if err := os.WriteFile(config, []byte(newer), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Check(ctx, options); errorCode(err) != "upgrade-required" || !strings.Contains(err.Error(), "upgrade trailguide to 0.3.0 or newer") {
		t.Fatalf("check: %v", err)
	}
	options.ToolVersion = "0.3.0"
	if _, err := Check(ctx, options); errorCode(err) == "upgrade-required" || err == nil || !strings.Contains(err.Error(), "futureSetting") {
		t.Fatalf("check with the minimum version: %v", err)
	}
}
