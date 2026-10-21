// Test the problems YAML reports for input that isn't one YAML document.

package decode_test

import (
	"errors"
	"testing"

	"github.com/fabricahq/trailguide/internal/decode"
	"github.com/fabricahq/trailguide/internal/errs"
)

func TestYAMLReportsTheParsersErrorAndPosition(t *testing.T) {
	for _, test := range []struct {
		name, input string
		firstLine   int
		want        string
	}{
		{"unquoted colon", "name: x\ndescription: Ratchet size: run it\n", 1, "invalid YAML at line 2, column 26: mapping values are not allowed in this context; quote a value that contains a colon followed by a space"},
		{"line offset", "name: x\ndescription: Ratchet size: run it\n", 2, "invalid YAML at line 3, column 26: mapping values are not allowed in this context; quote a value that contains a colon followed by a space"},
		{"unclosed flow sequence", "include: [a\n", 1, "invalid YAML at line 2, column 1: did not find expected ',' or ']'"},
		{"second document", "a: 1\n---\nb: 2\n", 1, "expected one YAML document"},
		{"syntax error in a second document", "a: 1\n---\nb: [\n", 1, "invalid YAML at line 4, column 1: did not find expected node content"},
		{"empty", "", 1, "expected one YAML document"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := decode.YAMLAt([]byte(test.input), "config.yaml", test.firstLine)
			var invalid errs.ValidationError
			if !errors.As(err, &invalid) || invalid.ValidationLocation() != "config.yaml" || invalid.ValidationProblem() != test.want {
				t.Fatalf("YAMLAt = %v; want config.yaml: %s", err, test.want)
			}
		})
	}
}
