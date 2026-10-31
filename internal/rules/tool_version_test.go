// Verify parsing, reading, and writing of minTrailguideVersion.

package rules_test

import (
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/rules"
)

// TestReleaseVersion accepts release versions and treats development and prerelease builds as having none.
func TestReleaseVersion(t *testing.T) {
	for running, want := range map[string]bool{"0.9.0": true, "10.0.1": true, "0.0.0-development": false, "1.0.0-rc.1": false, "v0.9.0": false, "": false} {
		if _, ok := rules.ReleaseVersion(running); ok != want {
			t.Errorf("ReleaseVersion(%q) = %v, want %v", running, ok, want)
		}
	}
	older, _ := rules.ReleaseVersion("0.9.10")
	newer, _ := rules.ReleaseVersion("0.10.0")
	if older.Compare(newer) != -1 || newer.Compare(older) != 1 || older.Compare(older) != 0 {
		t.Fatal("versions compare as text, not numbers")
	}
}

// TestConfigurationMinimumVersion parses minTrailguideVersion and rejects anything but a release version.
func TestConfigurationMinimumVersion(t *testing.T) {
	config, err := rules.ParseConfigurationYAML([]byte("schemaVersion: 2\nminTrailguideVersion: 0.9.0\nsources: {}\n"))
	if err != nil || config.MinimumVersion == nil || config.MinimumVersion.String() != "0.9.0" {
		t.Fatalf("%+v %v", config.MinimumVersion, err)
	}
	if config, err := rules.ParseConfigurationYAML([]byte("schemaVersion: 2\nsources: {}\n")); err != nil || config.MinimumVersion != nil {
		t.Fatalf("%+v %v", config.MinimumVersion, err)
	}
	for _, value := range []string{"1", "0.9", "v0.9.0", "0.9.0-rc.1", "null", "[0.9.0]"} {
		if _, err := rules.ParseConfigurationYAML([]byte("schemaVersion: 2\nminTrailguideVersion: " + value + "\nsources: {}\n")); err == nil || !strings.Contains(err.Error(), "minTrailguideVersion") {
			t.Errorf("%s: %v", value, err)
		}
	}
}

// TestMinimumVersion reads the field alone, so a configuration with fields or values this version doesn't know still
// reports it, and reports none for a configuration it can't read.
func TestMinimumVersion(t *testing.T) {
	for _, other := range []string{"futureSetting: true", "futureSetting: 2026-01-01", "futureSetting: .inf"} {
		if version, ok := rules.MinimumVersion([]byte("schemaVersion: 3\nminTrailguideVersion: 2.0.0\n" + other + "\n")); !ok || version.String() != "2.0.0" {
			t.Fatalf("%s: %v %v", other, version, ok)
		}
	}
	for _, input := range []string{"", "schemaVersion: 2\nsources: {}\n", "minTrailguideVersion: next\n", "<<<<<<< HEAD\n", "- 2.0.0\n"} {
		if _, ok := rules.MinimumVersion([]byte(input)); ok {
			t.Errorf("%q reported a minimum", input)
		}
	}
}

// TestSetConfigurationMinimumVersion adds the field after schemaVersion or replaces it, keeping comments.
func TestSetConfigurationMinimumVersion(t *testing.T) {
	version, _ := rules.ReleaseVersion("0.10.0")
	for input, want := range map[string]string{
		"# Project rules.\nschemaVersion: 2\nsources: {} # none yet\n":          "# Project rules.\nschemaVersion: 2\nminTrailguideVersion: 0.10.0\nsources: {} # none yet\n",
		"schemaVersion: 2\nminTrailguideVersion: 0.9.0 # raised\nsources: {}\n": "schemaVersion: 2\nminTrailguideVersion: 0.10.0 # raised\nsources: {}\n",
	} {
		got, err := rules.SetConfigurationMinimumVersion([]byte(input), version)
		if err != nil || string(got) != want {
			t.Errorf("from:\n%s\ngot:\n%s\nwant:\n%s\n%v", input, got, want, err)
		}
	}
}

// TestUpgradeRequired refuses only a release older than the configuration's minimum.
func TestUpgradeRequired(t *testing.T) {
	config := []byte("schemaVersion: 2\nminTrailguideVersion: 0.10.0\nfutureSetting: true\n")
	for _, test := range []struct {
		config   []byte
		running  string
		required bool
	}{
		{config, "0.9.2", true},
		{config, "0.10.0", false},
		{config, "0.11.0", false},
		{config, "0.0.0-development", false},
		{config, "0.9.0-rc.1", false},
		{config, "", false},
		{nil, "0.9.2", false},
		{[]byte("schemaVersion: 2\n"), "0.9.2", false},
	} {
		problem, required := rules.UpgradeRequired(test.config, test.running)
		if required != test.required {
			t.Errorf("%q with %s: required %v, want %v", test.config, test.running, required, test.required)
		}
		if required && problem != ".trailguide/config.yaml: minTrailguideVersion is 0.10.0, but this is trailguide 0.9.2; upgrade trailguide to 0.10.0 or newer, then run the command again" {
			t.Errorf("problem: %s", problem)
		}
	}
}
