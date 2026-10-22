// Name the repository addresses and refs that gitsource validates as part of a configuration's sources.

package rules

import "github.com/fabricahq/trailguide/internal/gitsource"

type (
	// Repository identifies a Git source without rewriting its transport address; see gitsource.Repository.
	Repository = gitsource.Repository
	// RepositoryWeb holds public browsing endpoints for GitHub or GitLab; see gitsource.RepositoryWeb.
	RepositoryWeb = gitsource.RepositoryWeb
	// GitRef is a validated commit, tag, or branch; see gitsource.GitRef.
	GitRef = gitsource.GitRef
	// GitRefKind distinguishes a commit, a tag, and a branch; see gitsource.GitRefKind.
	GitRefKind = gitsource.GitRefKind
)

// The kinds of GitRef; see gitsource.GitRefCommit.
const (
	GitRefCommit = gitsource.GitRefCommit
	GitRefTag    = gitsource.GitRefTag
	GitRefBranch = gitsource.GitRefBranch
)

// LatestReleaseRef is the ref that takes a library's newest library release; see gitsource.LatestReleaseRef.
const LatestReleaseRef = gitsource.LatestReleaseRef

var (
	// ParseRepository validates an explicit Git address; see gitsource.ParseRepository.
	ParseRepository = gitsource.ParseRepository
	// ParseGitRef validates a commit, tag, or branch; see gitsource.ParseGitRef.
	ParseGitRef = gitsource.ParseGitRef
	// TagVersion validates a complete semantic version tag; see gitsource.TagVersion.
	TagVersion = gitsource.TagVersion
)
