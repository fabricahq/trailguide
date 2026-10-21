// Check change note parsing against independent fixtures, including every documented rejection.

package rules_test

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/fabricahq/trailguide/internal/errs"
	"github.com/fabricahq/trailguide/internal/rules"
)

// TestChangeNoteFixtures checks parsed notes and exact diagnostics.
func TestChangeNoteFixtures(t *testing.T) {
	data, err := os.ReadFile("testdata/change-notes/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		ID, Location, Input string
		Expected            struct {
			OK    bool
			Value json.RawMessage
			Error *struct{ Message, Location string }
		}
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, test := range cases {
		t.Run(test.ID, func(t *testing.T) {
			got, err := rules.ParseChangeNote([]byte(test.Input), test.Location)
			if !test.Expected.OK {
				var validation errs.ValidationError
				if !errors.As(err, &validation) || err.Error() != test.Expected.Error.Message || validation.ValidationLocation() != test.Expected.Error.Location {
					t.Fatalf("got %+v, %v; want %+v", got, err, test.Expected.Error)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var expected rules.ChangeNote
			if err := json.Unmarshal(test.Expected.Value, &expected); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, expected) {
				t.Fatalf("got %+v; want %+v", got, expected)
			}
		})
	}
}
