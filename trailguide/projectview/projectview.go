// Package projectview combines a Trail Guide project's configuration and lockfile into the rules the project holds
// and uses, so tools that read a project, such as a catalog that reads it from GitHub, and Trail Guide's own
// listing, agree on what each rule's state is.
//
// Read takes the bytes of .trailguide/config.yaml and .trailguide/lock.json and needs no other file, and no Git,
// filesystem, or network access. The configuration says what the project asks for: its sources, pins, exclusions,
// and replacements. The lockfile says what each source resolved to and the version of every rule the project holds.
// A View joins the two, with each relationship as its own field, since a rule can be pinned and have an update
// waiting at once. Updates then compares a view with a library's current rule versions.
//
// A view describes the rules of the project's sources and the authored rules that replace them. Other authored
// rules live only in the project's files, so a view doesn't list them.
//
// Invalid input fails with a *trailguide.ValidationError; a lockfile in a newer format fails with
// *trailguide.UnsupportedLockError. This is a public API that follows Trail Guide's own version, and may change
// before Trail Guide 1.0.0.
package projectview

import (
	"errors"
	"maps"
	"slices"
	"strings"

	"github.com/fabricahq/trailguide/internal/errs"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/trailguide"
)

// LocalSource is the source name in the project ID of an authored rule, such as local:techs/go/errors.
const LocalSource = "local"

// View is what a project holds of its sources and how it uses each rule. Sources are in name order; Rules are in
// project ID order.
type View struct {
	Sources []Source `json:"sources"`
	Rules   []Rule   `json:"rules"`
}

// Source is one source the project takes, as the lockfile records it.
type Source struct {
	// Name is the project's name for the source, which its rules' project IDs start with.
	Name string `json:"name"`
	// Kind is trailguide.KindLibrary or trailguide.KindNonLibrary.
	Kind       string `json:"kind"`
	Repository string `json:"repository"`
	// Ref is the ref the configuration named when the lockfile was written, such as release/4, latest-release, or
	// main, and Commit the full commit SHA it resolved to.
	Ref    string `json:"ref"`
	Commit string `json:"commit"`
	// Release is the library release the source is on, or 0 when it follows a branch or commit, or isn't a library.
	Release int `json:"release,omitempty"`
	// Groups are the rule groups the project includes from the source, sorted; empty for a source that isn't a
	// library.
	Groups []string `json:"groups"`
}

// Rule is one rule the project holds of a source, or an authored rule that replaces one.
type Rule struct {
	// ID is the rule's project ID, such as fabrica:techs/go/errors, or local:techs/go/errors for an authored rule.
	ID string `json:"id"`
	// Source names the source the rule comes from; it is empty for an authored rule.
	Source string `json:"source,omitempty"`
	// Path is the rule's ID in its source, or the authored rule's ID, such as techs/go/errors.
	Path string `json:"path"`
	// Version and Release are the rule's version and the library release that published it. Both are nil for an
	// authored rule and for a rule file with unreleased changes.
	Version *trailguide.RuleVersion `json:"version,omitempty"`
	Release *int                    `json:"release,omitempty"`
	// Commit is the commit the project's copy of the rule comes from; it is empty for an authored rule.
	Commit string `json:"commit,omitempty"`
	// Active is true when the configuration includes the rule in the project's guidance: a source's rule the
	// project holds and doesn't exclude, or an authored rule that replaces one. It describes the two files Read
	// takes, so it doesn't say whether the generated guidance is current, or whether a replacement's file exists and
	// builds.
	Active bool `json:"active"`
	// Pin is the project's pin of the rule, or nil.
	Pin *Pin `json:"pin,omitempty"`
	// Exclusion is the project's exclusion of the rule, by its ID or a selector that matches it, or nil.
	Exclusion *Exclusion `json:"exclusion,omitempty"`
	// ReplacedBy is the project ID of the authored rule agents receive instead, or empty.
	ReplacedBy string `json:"replacedBy,omitempty"`
	// Replaces describes, on an authored rule, the source's rule it replaces; it is nil on every other rule.
	Replaces *Replacement `json:"replaces,omitempty"`
	// Retired is true when the rule's library has retired it. A source's rule the project still holds while
	// retired stays until trailguide project update removes it, or a pin keeps it.
	Retired bool `json:"retired"`
}

// Pin keeps a rule at a published version, with the project's reason.
type Pin struct {
	Version trailguide.RuleVersion `json:"version"`
	Reason  string                 `json:"reason"`
}

// Exclusion leaves a rule out of the project's guidance, with the project's reason.
type Exclusion struct {
	Reason string `json:"reason"`
}

// Replacement is the source's rule an authored rule replaces: its project ID, and the library version the authored
// rule is based on, or nil when the configuration records none.
type Replacement struct {
	ID      string                  `json:"id"`
	BasedOn *trailguide.RuleVersion `json:"basedOn,omitempty"`
}

// Read returns the view of a project from its configuration, the bytes of .trailguide/config.yaml, and its lockfile,
// the bytes of .trailguide/lock.json. It fails when either is invalid, or when the lockfile doesn't record what the
// configuration asks for: it has no entry for a declared source, or an entry whose repository, ref, rule or skill
// selectors, or pinned versions differ from the source's, until trailguide project sync or update writes it again. It
// ignores lockfile entries for sources the configuration no longer declares. An exclusion or replacement changes no
// lockfile entry, so the view applies the configuration's, as the next build does.
func Read(config, lock []byte) (View, error) {
	configuration, err := rules.ParseConfigurationYAML(config)
	if err != nil {
		return View{}, translate(err)
	}
	locked, err := trailguide.ParseLock(lock)
	if err != nil {
		return View{}, err
	}
	view := View{Sources: []Source{}, Rules: []Rule{}}
	for _, source := range configuration.Sources {
		entry, ok := locked.Sources[source.Name]
		if !ok {
			return View{}, &trailguide.ValidationError{Location: trailguide.LockFile + ".sources." + source.Name, Problem: "the lockfile has no entry for this source; run trailguide project sync"}
		}
		if err := matchEntry(source, entry); err != nil {
			return View{}, err
		}
		view.Sources = append(view.Sources, Source{
			Name: source.Name, Kind: entry.Kind, Repository: entry.Repository, Ref: entry.Ref, Commit: entry.Commit,
			Release: entry.Release, Groups: append([]string{}, entry.Groups...),
		})
		retired := map[string]bool{}
		for _, id := range entry.RetiredRules {
			retired[id] = true
		}
		for _, path := range slices.Sorted(maps.Keys(entry.Rules)) {
			held := entry.Rules[path]
			rule := Rule{
				ID: source.Name + ":" + path, Source: source.Name, Path: path, Version: held.Version, Release: held.Release,
				Commit: entry.Commit, Active: true, Retired: retired[path],
			}
			if held.Release != nil {
				rule.Commit = entry.ReleaseCommits[*held.Release]
			}
			if pin, ok := source.Pins[path]; ok {
				rule.Pin = &Pin{Version: pin.Version, Reason: pin.Reason}
			}
			if exclusion, ok := source.Exclusion(path); ok {
				rule.Active = false
				rule.Exclusion = &Exclusion{Reason: exclusion.Reason}
				if exclusion.ReplacedBy != "" {
					rule.ReplacedBy = LocalSource + ":" + exclusion.ReplacedBy
				}
			}
			view.Rules = append(view.Rules, rule)
		}
		// A replacement comes from the configuration alone, so it stays when the source no longer holds the rule it
		// replaces, such as after an update removed a retired rule.
		for _, path := range slices.Sorted(maps.Keys(source.Exclude)) {
			if exclusion := source.Exclude[path]; exclusion.ReplacedBy != "" {
				view.Rules = append(view.Rules, Rule{
					ID: LocalSource + ":" + exclusion.ReplacedBy, Path: exclusion.ReplacedBy, Active: true,
					Replaces: &Replacement{ID: source.Name + ":" + path, BasedOn: exclusion.BasedOn},
				})
			}
		}
	}
	slices.SortFunc(view.Rules, func(a, b Rule) int { return strings.Compare(a.ID, b.ID) })
	return view, nil
}

// Updates is how far the rules a project holds of one library are behind the library's current versions, in three
// lists whose meanings differ, so a caller shouldn't add them together. Each list is in rule ID order.
type Updates struct {
	// Active lists the active rules the library has a newer version of, or has retired: trailguide project update
	// would change the guidance agents receive, unless the rule is pinned.
	Active []RuleUpdate `json:"active"`
	// ReplacementBasis lists the rules an authored rule replaces whose library version is newer than the one the
	// replacement is based on, or than the version the project holds when it records none: the authored rule may
	// need the library's changes.
	ReplacementBasis []RuleUpdate `json:"replacementBasis"`
	// Excluded lists the rules excluded without a replacement that have a newer version or were retired. Nothing the
	// project uses changes.
	Excluded []RuleUpdate `json:"excluded"`
}

// RuleUpdate is one rule with a newer version in its library, or that its library has retired.
type RuleUpdate struct {
	// ID is the rule's project ID, and Path its ID in its source.
	ID   string `json:"id"`
	Path string `json:"path"`
	// Held is the version the comparison starts from: the version the project holds, or the version a replacement is
	// based on. It is nil for a rule file with unreleased changes.
	Held *trailguide.RuleVersion `json:"held,omitempty"`
	// Current is the library's current version, or nil when the library has retired the rule.
	Current *trailguide.RuleVersion `json:"current,omitempty"`
	// Pinned is true when the project pins the rule, so updates keep its version.
	Pinned bool `json:"pinned"`
}

// Updates compares the rules the view holds of source with its library's current state: current, each current
// rule's version by its ID in the library, and retired, the IDs of the rules it has retired. A rule the project still
// holds after its library retired it is listed, whether the lockfile or retired says so, since trailguide project
// update would remove it unless it's pinned. A rule the library
// neither holds nor retired, such as one dropped from its history, isn't listed. A rule held at an unreleased version
// is listed when the library holds it at all, since any published version differs from it. Rules the library added
// aren't listed: the view can't tell whether the project's selectors would include them.
func (v View) Updates(source string, current map[string]trailguide.RuleVersion, retired map[string]bool) Updates {
	result := Updates{Active: []RuleUpdate{}, ReplacementBasis: []RuleUpdate{}, Excluded: []RuleUpdate{}}
	basedOn := map[string]*trailguide.RuleVersion{}
	for _, rule := range v.Rules {
		if rule.Replaces != nil {
			basedOn[rule.Replaces.ID] = rule.Replaces.BasedOn
		}
	}
	for _, rule := range v.Rules {
		if rule.Source != source {
			continue
		}
		held := rule.Version
		if based, replaced := basedOn[rule.ID]; replaced && based != nil {
			held = based
		}
		update := RuleUpdate{ID: rule.ID, Path: rule.Path, Held: held, Pinned: rule.Pin != nil}
		if version, ok := current[rule.Path]; ok {
			if held != nil && version.Compare(*held) <= 0 {
				continue
			}
			update.Current = &version
		} else if !retired[rule.Path] && !rule.Retired {
			continue
		}
		switch {
		case rule.Active:
			result.Active = append(result.Active, update)
		case rule.ReplacedBy != "":
			result.ReplacementBasis = append(result.ReplacementBasis, update)
		default:
			result.Excluded = append(result.Excluded, update)
		}
	}
	return result
}

// matchEntry checks that entry, a source's lockfile entry, records what source's configuration asks for: the same
// repository, ref, and rule and skill selectors, and each pinned rule at its pinned version, unless its library retired
// it. Otherwise the configuration changed since sync or update last wrote the entry.
func matchEntry(source rules.Source, entry trailguide.LockedSource) error {
	unsynced := &trailguide.ValidationError{
		Location: trailguide.LockFile + ".sources." + source.Name,
		Problem:  "the entry doesn't record what sources." + source.Name + " in the configuration asks for; run trailguide project sync, or trailguide project update " + source.Name + " for a changed ref",
	}
	ref, err := rules.ParseGitRef(entry.Ref, "ref")
	if err != nil || entry.Repository != source.Repository || !source.Ref.Equal(ref) ||
		!slices.Equal(entry.Selectors.Rules, source.Rules) || !slices.Equal(entry.Selectors.Skills, source.Skills) {
		return unsynced
	}
	var groups rules.GroupSelection
	if len(entry.Selectors.Groups) > 0 && groups.UnmarshalJSON(entry.Selectors.Groups) != nil || !groups.Equal(source.Groups) {
		return unsynced
	}
	for id, pin := range source.Pins {
		rule, held := entry.Rules[id]
		if held && (rule.Version == nil || *rule.Version != pin.Version) || !held && !slices.Contains(entry.RetiredRules, id) {
			return unsynced
		}
	}
	return nil
}

// translate returns err with an internal validation error replaced by a *trailguide.ValidationError with the same
// location and problem, so callers only see the public package's error types.
func translate(err error) error {
	var validation errs.ValidationError
	if errors.As(err, &validation) {
		return &trailguide.ValidationError{Location: validation.ValidationLocation(), Problem: validation.ValidationProblem()}
	}
	return err
}
