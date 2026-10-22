// Validate the library section of an authored YAML configuration without filesystem side effects.

package rules_test

import (
	"testing"

	"github.com/fabricahq/trailguide/internal/rules"
)

func TestYAMLMetadata(t *testing.T) {
	manifest := "schemaVersion: 2\nsources: {}\n# Library terms\nlibrary:\n  formatVersion: 1\n  license:\n    spdxExpression: MIT\n    file: LICENSE.md\n    notices:\n      - NOTICE.md\n"
	library, err := rules.ParseLibraryManifest([]byte(manifest), "library/.trailguide/config.yaml")
	if err != nil || library == nil || library.License == nil || *library.License.SPDXExpression != "MIT" || len(library.License.AttributionFiles) != 1 {
		t.Fatal(library, err)
	}
}

func TestYAMLMetadataRejectsAmbiguousInput(t *testing.T) {
	for name, input := range map[string]string{
		"duplicate": "schemaVersion: 2\nsources: {}\nlibrary: {formatVersion: 1}\nlibrary: {formatVersion: 1}\n", "multiple": "schemaVersion: 2\nsources: {}\nlibrary: {formatVersion: 1}\n---\nschemaVersion: 2\n",
		"anchor": "schemaVersion: 2\nsources: {}\nlibrary: &library {}", "alias": "schemaVersion: 2\nsources: {}\nlibrary: *library", "tag": "schemaVersion: 2\nsources: {}\nlibrary: !!map {}",
		"wrong-type": "schemaVersion: 2\nsources: {}\nlibrary: false", "invalid-utf8": "schemaVersion: 2\nsources: {}\nlibrary: {formatVersion: 1}\n#\xff", "empty": "", "nonmapping": "[]",
		"nested-duplicate": "schemaVersion: 2\nsources: {}\nlibrary:\n  formatVersion: 1\n  license: {file: LICENSE.md, file: NOTICE.md, notices: []}",
	} {
		t.Run("library/"+name, func(t *testing.T) {
			if _, err := rules.ParseLibraryManifest([]byte(input), "library/.trailguide/config.yaml"); err == nil {
				t.Fatal("accepted invalid metadata")
			}
		})
	}
}
