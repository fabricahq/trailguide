// Initialize library repositories and publish authored definitions without replacing existing content.

package library

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v4"

	"github.com/fabricahq/trailguide/internal/filetxn"
	"github.com/fabricahq/trailguide/internal/gitexec"
	"github.com/fabricahq/trailguide/internal/librarytree"
	"github.com/fabricahq/trailguide/internal/rules"
	"github.com/fabricahq/trailguide/trailguide"
)

// AuthoringResult lists the absolute paths of the library files an operation created and changed, and any
// post-commit cleanup warnings.
type AuthoringResult struct {
	Added   []string `json:"added"`
	Changed []string `json:"changed"`
	// LicenseDeclared describes the manifest after initialization, and HasGroups whether the library already had a
	// technology or practice group.
	LicenseDeclared bool     `json:"-"`
	HasGroups       bool     `json:"-"`
	Warnings        []string `json:"warnings"`
}

// Written returns every path the operation wrote: its created files, then its changed ones.
func (r AuthoringResult) Written() []string { return append(slices.Clone(r.Added), r.Changed...) }

// Options locates the library itself, independently of any consumer project.
type Options struct {
	Directory string
	// Git runs Git in the library's own repository, to read its library releases; zero values use Git from PATH.
	Git gitexec.Options
	// Curated reads the sources the library curates skills from, so check can find their skills; nil leaves them
	// unchecked.
	Curated CuratedReader
	// ReadSkill reads a skill from a source the library declares, which a fork copies.
	ReadSkill SkillReader
	// Forks compares the skills the library forked with the sources it forked them from, so check and release can
	// list what the sources changed; nil leaves them uncompared.
	Forks ForkReader
}

// Terms contains publisher-supplied text; nil Notice omits a notice while an empty value preserves an empty file.
type Terms struct {
	SPDXExpression string
	License        string
	Notice         *string
}

// RuleOptions distinguishes an explicit body from a marked canonical draft in an existing group.
type RuleOptions struct {
	Options
	Body *string
}

// libraryReadme introduces publisher workflows; CLI tests execute its command examples.
//
//go:embed library-guide.md
var libraryReadme string

// checkWorkflow runs trailguide library check on pull requests; checkWorkflowFor fills in checkWorkflowVersion.
//
//go:embed check-workflow.yml
var checkWorkflow string

// checkWorkflowVersion marks where checkWorkflow's install step sets the Trail Guide version it installs.
const checkWorkflowVersion = "TRAILGUIDE_VERSION"

// latestVersion is the shell command that the install step of a development build's workflow runs to find the
// version of the latest Trail Guide release, without its v prefix.
const latestVersion = `$(gh release view --repo fabricahq/trailguide --json tagName --jq '.tagName | ltrimstr("v")')`

// developmentVersion prefixes the versions of builds that no release published: development builds and
// unpublished candidates, such as 0.0.0-development and 0.0.0-dev.g0123456789ab.
const developmentVersion = "0.0.0-"

// checkWorkflowPath is the GitHub Actions workflow library init writes.
const checkWorkflowPath = ".github/workflows/trailguide.yml"

// openLibrary opens the library repository's root, enforcing the same final-root no-link policy as project
// authoring. It fails, closing the root, when the repository is in the library layout before version 2 or keeps
// its Trail Guide directory under the name Code Rules used.
func openLibrary(ctx context.Context, options Options, create bool) (*os.Root, error) {
	directory := options.Directory
	if directory == "" {
		directory = "."
	}
	var root *os.Root
	var err error
	if create {
		root, err = filetxn.Create(ctx, directory)
	} else {
		root, err = filetxn.Open(ctx, directory)
	}
	if errors.Is(err, os.ErrNotExist) && !create {
		return nil, failure("needs-init", directory+": missing library directory; run trailguide library init first", err)
	}
	if err != nil {
		return nil, err
	}
	if err := requireLibraryLayout(ctx, root); err != nil {
		root.Close()
		return nil, err
	}
	if err := requireCurrentDirectory(root); err != nil {
		root.Close()
		return nil, err
	}
	return root, nil
}

// manifestPath is where the manifest lives in the library's repository.
var manifestPath = librarytree.RepositoryPath(librarytree.Manifest)

// declaredTerms validates the configuration in manifest, which must have a library section, before reading the
// contained term files it declares, using planned bytes, by library path, during initialization. The returned
// inventory holds the manifest and terms by library path.
func declaredTerms(ctx context.Context, root *os.Root, manifest []byte, planned map[string][]byte) (map[string][]byte, *trailguide.LicenseDeclaration, error) {
	declaration, err := rules.ParseLibraryManifest(manifest, manifestPath)
	if err != nil {
		return nil, nil, err
	}
	if declaration == nil {
		return nil, nil, failure("needs-init", manifestPath+" has no library section, so this repository isn't a library; run trailguide library init to add one", nil)
	}
	inventory := map[string][]byte{librarytree.Manifest: manifest}
	for _, name := range rules.LicensePaths(declaration.License) {
		data, ok := planned[name]
		if !ok {
			file := librarytree.RepositoryPath(name)
			if err = checkParents(root, file); err != nil {
				return nil, nil, err
			}
			data, err = filetxn.ReadOptional(ctx, root, file)
			if err != nil {
				return nil, nil, err
			}
			if data == nil {
				return nil, nil, failure("missing-license", file+": missing declared license or notice", nil)
			}
		}
		inventory[name] = data
	}
	return inventory, declaration.License, nil
}

// checkParents refuses observed symlinks in every term-file parent without traversing unrelated repository files.
func checkParents(root *os.Root, name string) error {
	parts := strings.Split(name, "/")
	for i := 1; i < len(parts); i++ {
		prefix := strings.Join(parts[:i], "/")
		info, err := root.Lstat(prefix)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return failure("unsafe-path", prefix+": expected directories without symlinks", nil)
		}
	}
	return nil
}

// libraryManifest requires an initialized manifest and valid declared terms.
func libraryManifest(ctx context.Context, root *os.Root) (map[string][]byte, *trailguide.LicenseDeclaration, error) {
	data, err := readOptionalBelow(ctx, root, manifestPath)
	if err != nil {
		return nil, nil, err
	}
	if data == nil {
		return nil, nil, failure("needs-init", "missing "+manifestPath+"; run trailguide library init first", nil)
	}
	return declaredTerms(ctx, root, data, nil)
}

// Initialize creates missing scaffolding and explicit terms without overwriting existing terms, README, or check
// workflow. It adds a library section to .trailguide/config.yaml, creating the configuration when it's missing and
// keeping an existing one's sources and comments; an existing library section stays unchanged. The workflow installs
// trailguideVersion, an unprefixed version such as 0.2.0, from the Trail Guide release tagged with it, or the latest
// release for a 0.0.0- development build.
func Initialize(ctx context.Context, options Options, terms *Terms, trailguideVersion string) (AuthoringResult, error) {
	workflow, err := checkWorkflowFor(trailguideVersion)
	if err != nil {
		return AuthoringResult{}, err
	}
	root, err := openLibrary(ctx, options, true)
	if err != nil {
		return AuthoringResult{}, err
	}
	defer root.Close()
	dir, err := openTrailGuideDirectory(ctx, root, true)
	if err != nil {
		return AuthoringResult{}, err
	}
	defer dir.Close()
	var license *trailguide.LicenseDeclaration
	changes, err := filetxn.EditIn(ctx, dir, root, func() ([]filetxn.File, error) {
		var files []filetxn.File
		existing, err := filetxn.ReadOptional(ctx, root, manifestPath)
		if err != nil {
			return nil, err
		}
		manifest := existing
		var declared *rules.Library
		if existing != nil {
			if declared, err = rules.ParseLibraryManifest(existing, manifestPath); err != nil {
				return nil, err
			}
		}
		if declared != nil && terms != nil {
			return nil, failure("already-initialized", manifestPath+" already has a library section; edit its license declaration there", nil)
		}
		planned := map[string][]byte{}
		if declared == nil {
			section := map[string]any{}
			if terms != nil {
				notices := []string{}
				if terms.Notice != nil {
					notices = append(notices, "NOTICE.md")
				}
				section["license"] = map[string]any{"spdxExpression": terms.SPDXExpression, "file": "LICENSE.md", "notices": notices}
			}
			if existing == nil {
				manifest, err = yamlText(struct {
					SchemaVersion int            `yaml:"schemaVersion"`
					Sources       map[string]any `yaml:"sources"`
					Library       map[string]any `yaml:"library"`
				}{2, map[string]any{}, section})
			} else {
				manifest, err = rules.AppendConfigurationLibrary(existing, section)
			}
			if err != nil {
				return nil, err
			}
			files = append(files, filetxn.File{Path: manifestPath, Content: manifest, Previous: existing})
		}
		if terms != nil {
			planned["LICENSE.md"] = []byte(terms.License)
			files = append(files, filetxn.File{Path: "LICENSE.md", Content: planned["LICENSE.md"]})
			if terms.Notice != nil {
				planned["NOTICE.md"] = []byte(*terms.Notice)
				files = append(files, filetxn.File{Path: "NOTICE.md", Content: planned["NOTICE.md"]})
			}
		}
		_, license, err = declaredTerms(ctx, root, manifest, planned)
		if err != nil {
			return nil, err
		}
		readme, err := filetxn.ReadOptional(ctx, root, "README.md")
		if err != nil {
			return nil, err
		}
		if readme == nil {
			files = append(files, filetxn.File{Path: "README.md", Content: []byte(libraryReadme)})
		}
		existingWorkflow, err := readOptionalBelow(ctx, root, checkWorkflowPath)
		if err != nil {
			return nil, err
		}
		if existingWorkflow == nil {
			files = append(files, filetxn.File{Path: checkWorkflowPath, Content: workflow})
		}
		return files, nil
	})
	if err != nil {
		return AuthoringResult{}, err
	}
	result, err := authoringResult(changes, err)
	result.LicenseDeclared = license != nil
	for _, group := range []string{"practices", "techs"} {
		if entries, readErr := fs.ReadDir(root.FS(), librarytree.RepositoryPath(group)); readErr == nil && len(entries) > 0 {
			result.HasGroups = true
		}
	}
	return result, err
}

// checkWorkflowFor returns the check workflow pinned to version, which must be a complete semantic version
// without a v prefix, so it can't change the workflow's structure. Its install step downloads and verifies only
// that version's archive. A development build has no release to pin, so its workflow finds the latest release's
// version first and then installs exactly that one.
func checkWorkflowFor(version string) ([]byte, error) {
	if canonical, err := rules.TagVersion(version, "version"); err != nil || canonical != version {
		return nil, failure("invalid-version", "Trail Guide version "+strconv.Quote(version)+" is not a complete semantic version", err)
	}
	installed := version
	if strings.HasPrefix(version, developmentVersion) {
		installed = latestVersion
	}
	return []byte(strings.ReplaceAll(checkWorkflow, checkWorkflowVersion, installed)), nil
}

// readOptionalBelow reads a file whose parent directories may be missing; nil means the file is absent.
// Observed symlinks in its parents fail.
func readOptionalBelow(ctx context.Context, root *os.Root, name string) ([]byte, error) {
	if err := checkParents(root, name); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return filetxn.ReadOptional(ctx, root, name)
}

// editLibrary revalidates manifest and the target category under the writer of the Trail Guide directory before
// publication; prepare returns files by repository path.
func editLibrary(ctx context.Context, options Options, group string, prepare func(*os.Root) ([]filetxn.File, error)) (AuthoringResult, error) {
	root, err := openLibrary(ctx, options, false)
	if err != nil {
		return AuthoringResult{}, err
	}
	defer root.Close()
	dir, err := openTrailGuideDirectory(ctx, root, false)
	if err != nil {
		return AuthoringResult{}, err
	}
	defer dir.Close()
	changes, err := filetxn.EditIn(ctx, dir, root, func() ([]filetxn.File, error) {
		if _, _, err := libraryManifest(ctx, root); err != nil {
			return nil, err
		}
		if _, err := filetxn.ReadTree(ctx, root, librarytree.RepositoryPath(strings.Split(group, "/")[0])); err != nil {
			return nil, err
		}
		return prepare(root)
	})
	return authoringResult(changes, err)
}

// AddGroup creates complete metadata without changing existing library content.
func AddGroup(ctx context.Context, id string, metadata trailguide.GroupMetadata, options Options) (AuthoringResult, error) {
	if err := librarytree.ValidateGroupID(id, "group"); err != nil {
		return AuthoringResult{}, err
	}
	data, err := rules.RenderGroup(metadata)
	if err != nil {
		return AuthoringResult{}, err
	}
	return editLibrary(ctx, options, id, func(root *os.Root) ([]filetxn.File, error) {
		if err := checkNewGroup(ctx, root, id); err != nil {
			return nil, err
		}
		return groupFiles(librarytree.RepositoryPath(id), id, data), nil
	})
}

// hasGroupMetadata validates an existing group definition without creating it.
func hasGroupMetadata(ctx context.Context, root *os.Root, id string) (bool, error) {
	file := librarytree.RepositoryPath(id + "/_group.yaml")
	if err := checkParents(root, file); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	data, err := filetxn.ReadOptional(ctx, root, file)
	if err != nil || data == nil {
		return false, err
	}
	_, err = trailguide.ParseGroupMetadata(data, id)
	return err == nil, err
}

// AddRule creates supplied guidance or a marked unfinished canonical draft, in an existing group.
func AddRule(ctx context.Context, id string, metadata rules.RuleMetadata, options RuleOptions) (AuthoringResult, error) {
	group, err := librarytree.GroupFromPath(id+".md", "rule")
	if err != nil {
		return AuthoringResult{}, err
	}
	data, err := rules.RenderRule(id, metadata, options.Body)
	if err != nil {
		return AuthoringResult{}, err
	}
	return editLibrary(ctx, options.Options, group, func(root *os.Root) ([]filetxn.File, error) {
		if err := checkNewRule(ctx, root, id, options.Options); err != nil {
			return nil, err
		}
		return []filetxn.File{{Path: librarytree.RepositoryPath(id + ".md"), Content: data}}, nil
	})
}

// authoringResult returns published paths and cleanup warnings only after a successful write.
func authoringResult(changes filetxn.Changes, err error) (AuthoringResult, error) {
	if err != nil {
		return AuthoringResult{}, err
	}
	return AuthoringResult{Added: changes.Added, Changed: changes.Changed, Warnings: changes.Warnings}, nil
}

func yamlText(value any) ([]byte, error) {
	var out bytes.Buffer
	e := yaml.NewEncoder(&out)
	e.SetIndent(2)
	if err := e.Encode(value); err != nil {
		return nil, err
	}
	return out.Bytes(), e.Close()
}

func failure(code, problem string, cause error) error {
	return &filetxn.Error{Code: code, Problem: problem, Cause: cause}
}

func groupFiles(directory, id string, metadata []byte) []filetxn.File {
	instructions := fmt.Sprintf("## Add or edit rules\n\nRun commands from the repository root, four directories above this folder.\nUse `trailguide library add rule %s/<rule-name>` to add a rule; run `trailguide library add rule --help` for metadata and body options.\nEdit existing rule files directly, then run `trailguide library check`. Resolve errors before committing or publishing the library.\n\nA consuming project selects this group in its configuration and runs sync. Its generated/RULES.md identifies the adopted rules after exclusions and replacements.\n", id)
	return []filetxn.File{{Path: path.Join(directory, "_group.yaml"), Content: metadata}, {Path: path.Join(directory, "README.md"), Content: rules.GroupGuide(id, instructions)}}
}
