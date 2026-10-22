// Exercise repository identities, safe diagnostics, and file links through the public API.

package gitsource_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/fabricahq/trailguide/internal/gitsource"
)

// TestRepositorySharedExpectations checks exact outputs and typed, zero-result failures.
func TestRepositorySharedExpectations(t *testing.T) {
	data, err := os.ReadFile("testdata/repositories/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		ID, Operation, Location string
		Input                   json.RawMessage
		Expected                struct {
			OK    bool
			Value json.RawMessage
			Error *struct{ Name, Message, Location string }
		}
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, test := range cases {
		// Compare one public invocation with its independently authored expectation.
		t.Run(test.ID, func(t *testing.T) {
			original := bytes.Clone(test.Input)
			if test.Operation == "repositoryFile" {
				var input struct {
					Repository   json.RawMessage
					Commit, Path string
					Image        bool
				}
				if err := json.Unmarshal(test.Input, &input); err != nil {
					t.Fatal(err)
				}
				repository, err := gitsource.ParseRepository(input.Repository, test.Location)
				if !test.Expected.OK {
					var validation *gitsource.ValidationError
					if !errors.As(err, &validation) || err.Error() != test.Expected.Error.Message || validation.Location != test.Expected.Error.Location {
						t.Fatalf("got %v; want %s", err, test.Expected.Error.Message)
					}
					if !reflect.DeepEqual(repository, gitsource.Repository{}) {
						t.Fatalf("failure returned partial data: %#v", repository)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				var expected *string
				if err := json.Unmarshal(test.Expected.Value, &expected); err != nil {
					t.Fatal(err)
				}
				link, known := repository.FileURL(input.Commit, input.Path, input.Image)
				if known != (expected != nil) || (expected != nil && link != *expected) || (!known && link != "") {
					t.Fatalf("got %q, %v; expected %s", link, known, test.Expected.Value)
				}
				return
			}
			got, err := gitsource.ParseRepository(test.Input, test.Location)
			if !bytes.Equal(test.Input, original) {
				t.Fatal("parser changed the caller's address")
			}
			if test.Expected.OK {
				var expected gitsource.Repository
				if decodeErr := json.Unmarshal(test.Expected.Value, &expected); decodeErr != nil {
					t.Fatal(decodeErr)
				}
				if err != nil || !reflect.DeepEqual(got, expected) {
					t.Fatalf("got %#v, %v; want %#v", got, err, expected)
				}
				return
			}
			var validation *gitsource.ValidationError
			if !errors.As(err, &validation) || err.Error() != test.Expected.Error.Message || validation.Location != test.Expected.Error.Location {
				t.Fatalf("got %v; want %s", err, test.Expected.Error.Message)
			}
			if !reflect.DeepEqual(got, gitsource.Repository{}) {
				t.Fatalf("failure returned partial data: %#v", got)
			}
		})
	}
}

// TestRepositoryMalformedJSON checks direct-call syntax failures outside the lab envelope.
func TestRepositoryMalformedJSON(t *testing.T) {
	for _, input := range []string{"", `"unterminated`, `"\u0"`, `true`, `"a" "b"`} {
		_, err := gitsource.ParseRepository(json.RawMessage(input), "repo")
		if err == nil || err.Error() != "repo: expected nonempty text" {
			t.Fatalf("got %v for %q", err, input)
		}
	}
}

// FuzzParseRepository checks untrusted JSON for panics, partial results, and secret-bearing diagnostics.
func FuzzParseRepository(f *testing.F) {
	for _, input := range []string{`"https://github.com/team/rules.git"`, `"git@host.example:/Rules.git"`, `"ssh://git:secret@host.example/rules"`, `"https://example.org/\ud800"`, `null`, `"\\"`} {
		f.Add(input)
	}
	// Arbitrary bytes must produce a complete repository or a typed validation failure.
	f.Fuzz(func(t *testing.T, input string) {
		got, err := gitsource.ParseRepository(json.RawMessage(input), "repo")
		if err != nil {
			var validation *gitsource.ValidationError
			if !errors.As(err, &validation) || !reflect.DeepEqual(got, gitsource.Repository{}) {
				t.Fatalf("invalid failure result: %#v, %v", got, err)
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatal("diagnostic exposed a credential")
			}
		} else if got.Identity == "" {
			t.Fatal("successful parse returned no identity")
		}
	})
}
