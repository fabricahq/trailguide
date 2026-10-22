// List a synced project's rules before and after it replaces one with a fork, and alongside its other authored rules.

package project

import (
	"context"
	"slices"
	"strings"
	"testing"
)

func TestListRules_ShowsEachRulesVersionAndHowTheProjectUsesIt(t *testing.T) {
	f := newForkFixture(t, "")
	root, err := openProject(context.Background(), f.options, false)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, root, "rules/techs/go/naming.md", forkedRule("Name things."))
	root.Close()
	if _, err := f.fork(t, "techs/go/errors", "team@1.0.0", "Our services need the original wording."); err != nil {
		t.Fatal(err)
	}
	listing, err := ListRules(context.Background(), f.options)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, rule := range listing.Rules {
		row := rule.ID
		if rule.Version != nil {
			row += " " + rule.Version.String()
		}
		if rule.Active {
			row += " active"
		}
		if rule.ReplacedBy != "" {
			row += " replaced by " + rule.ReplacedBy
		}
		if rule.Replaces != nil {
			row += " replaces " + rule.Replaces.ID + " based on " + rule.Replaces.BasedOn.String()
		}
		got = append(got, row)
	}
	want := []string{
		"local:techs/go/errors active replaces team:techs/go/errors based on 1.0.0",
		"local:techs/go/naming active",
		"team:techs/go/errors 1.0.0 replaced by local:techs/go/errors",
		"team:techs/go/licensed 1.0.0 active",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("listing:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
