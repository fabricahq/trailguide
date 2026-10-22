// Validate explicit Git addresses and derive links without fetching repositories.

package gitsource

import (
	"encoding/json"
	neturl "net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/nlnwa/whatwg-url/url"

	"github.com/fabricahq/trailguide/internal/decode"
)

// Repository identifies a Git source without rewriting its transport address.
type Repository struct {
	Identity string `json:"identity"`
	// Web is nil when the host or transport has no recognized browser convention.
	Web *RepositoryWeb `json:"web"`
}

// RepositoryWeb holds public browsing endpoints for GitHub or GitLab.
type RepositoryWeb struct {
	Root string `json:"root"`
	Tree string `json:"tree"`
	File string `json:"file"`
	Raw  string `json:"raw"`
}

var (
	repositoryURL   = regexp.MustCompile(`^(https|ssh)://([^/]+)/(.+)$`)
	repositorySCP   = regexp.MustCompile(`^([A-Za-z0-9_][A-Za-z0-9_.-]*)@(\[[^\]]+\]|[^:/]+):(.+)$`)
	repositoryUser  = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)
	repositoryLabel = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)
	repositoryIPv6  = regexp.MustCompile(`^\[[a-f0-9:.]+\]$`)
)

// ParseRepository accepts a JSON string containing an explicit HTTPS, SSH, or
// user@host:path address. JSON input preserves lone surrogate escapes for validation.
// It rejects credentials and ambiguous paths, returning a zero value on failure.
// Parsing does not establish that a repository exists or is accessible.
func ParseRepository(input json.RawMessage, location string) (Repository, error) {
	var address string
	if json.Unmarshal(input, &address) != nil || strings.TrimFunc(address, decode.IsSpace) == "" {
		return Repository{}, invalid(location, "expected nonempty text")
	}
	if !decode.ValidUnicode(input) || strings.ContainsFunc(address, unsafeRepositoryCharacter) {
		return Repository{}, invalid(location, "repository must not contain whitespace, backslashes, query parameters, or fragments")
	}
	uri := repositoryURL.FindStringSubmatch(address)
	var scp []string
	if uri == nil {
		scp = repositorySCP.FindStringSubmatch(address)
	}
	var rawPath, urlText string
	var err error
	switch {
	case uri != nil:
		urlText = address
		rawPath, err = repositoryPath(uri[3], location, true)
	case scp != nil:
		absolute := strings.HasPrefix(scp[3], "/")
		rawPath, err = repositoryPath(strings.TrimPrefix(scp[3], "/"), location, false)
		if absolute {
			rawPath = "/" + rawPath
		}
		urlText = "ssh://" + scp[1] + "@" + scp[2] + "/"
	default:
		return Repository{}, invalid(location, "repository must be an explicit HTTPS URL, ssh:// URL, or user@host:path address; owner/name shorthand is unsupported")
	}
	if err != nil {
		return Repository{}, err
	}
	parsed, err := url.Parse(urlText)
	if err != nil {
		return Repository{}, invalid(location, "invalid Git repository URL")
	}
	badAuthority := false
	if uri != nil && strings.Contains(uri[2], "@") {
		before, _, _ := strings.Cut(uri[2], "@")
		badAuthority = parsed.Username() == "" || strings.Contains(before, ":")
	}
	if parsed.Hostname() == "" || parsed.Password() != "" || badAuthority ||
		(parsed.Scheme() == "https" && parsed.Username() != "") ||
		(parsed.Username() != "" && !repositoryUser.MatchString(parsed.Username())) {
		return Repository{}, invalid(location, "repository requires a host and must not embed credentials; SSH may specify a username")
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if !repositoryIPv6.MatchString(host) {
		for _, label := range strings.Split(host, ".") {
			if !repositoryLabel.MatchString(label) {
				return Repository{}, invalid(location, "repository hostname must be a DNS name or IP address")
			}
		}
	}
	standard := parsed.Port() == ""
	if parsed.Scheme() == "ssh" {
		standard = (parsed.Port() == "" || parsed.Port() == "22") && parsed.Username() == "git"
	}
	browserPath := rawPath
	if scp != nil {
		browserPath = encodeRepositoryPath(strings.TrimPrefix(rawPath, "/"), true)
	}
	path := strings.TrimSuffix(browserPath, ".git")
	segments := strings.Split(path, "/")
	known := standard && !strings.HasSuffix(path, "/") &&
		((host == "github.com" && len(segments) == 2) || (host == "gitlab.com" && len(segments) >= 2))
	if known {
		identityPath := path
		if host == "github.com" {
			identityPath = strings.ToLower(path)
		}
		root := "https://" + host + "/" + path
		web := &RepositoryWeb{Root: root, Tree: root + "/-/tree", File: root + "/-/blob", Raw: root + "/-/raw"}
		if host == "github.com" {
			web = &RepositoryWeb{Root: root, Tree: root + "/tree", File: root + "/blob", Raw: "https://raw.githubusercontent.com/" + path}
		}
		return Repository{Identity: host + "/" + identityPath, Web: web}, nil
	}
	user, port := "", ""
	if parsed.Username() != "" {
		user = parsed.Username() + "@"
	}
	if parsed.Port() != "" {
		port = ":" + parsed.Port()
	}
	identity := parsed.Protocol() + "//" + user + host + port + "/" + rawPath
	if scp != nil {
		identity = user + host + ":" + rawPath
	}
	return Repository{Identity: identity}, nil
}

// FileURL returns a file or raw-content link, or false for an unrecognized web host.
// Callers own commit and path validation; commit should be a resolved commit ID.
// Each path segment uses the reference's encodeURIComponent escaping.
func (r Repository) FileURL(commit, path string, raw bool) (string, bool) {
	if r.Web == nil {
		return "", false
	}
	base := r.Web.File
	if raw {
		base = r.Web.Raw
	}
	return base + "/" + commit + "/" + encodeRepositoryPath(path, false), true
}

// unsafeRepositoryCharacter rejects ambiguous separators, whitespace, and controls before URL parsing.
func unsafeRepositoryCharacter(r rune) bool {
	return decode.IsSpace(r) || r < 32 || r == 127 || r == '\\' || r == '?' || r == '#'
}

// repositoryPath validates authored segments before URL normalization can erase traversal.
// URL paths decode once and reject residual percent signs; SCP paths keep literal percent signs.
func repositoryPath(path, location string, uri bool) (string, error) {
	parts := strings.Split(strings.TrimSuffix(path, "/"), "/")
	for i, part := range parts {
		decoded := part
		if uri {
			var err error
			decoded, err = neturl.PathUnescape(part)
			if err != nil || !utf8.ValidString(decoded) {
				return "", invalid(location, "repository path contains invalid percent encoding")
			}
		}
		if decoded == "" || decoded == "." || decoded == ".." || strings.ContainsFunc(decoded, unsafeRepositorySegment) || (uri && strings.Contains(decoded, "%")) {
			return "", invalid(location, "repository path contains an empty, dot, or unsafe segment")
		}
		if uri {
			parts[i] = encodeRepositorySegment(decoded, true)
		}
	}
	return strings.Join(parts, "/"), nil
}

// unsafeRepositorySegment identifies decoded separators and ASCII control characters.
func unsafeRepositorySegment(r rune) bool {
	return r == '/' || r == '\\' || r < 32 || r == 127
}

// encodeRepositoryPath escapes each segment independently, retaining slash boundaries.
func encodeRepositoryPath(path string, markdown bool) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		parts[i] = encodeRepositorySegment(part, markdown)
	}
	return strings.Join(parts, "/")
}

// encodeRepositorySegment matches encodeURIComponent, optionally escaping Markdown punctuation too.
func encodeRepositorySegment(text string, markdown bool) string {
	const hex = "0123456789ABCDEF"
	var result strings.Builder
	for i := 0; i < len(text); i++ {
		c := text[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || strings.ContainsRune("-_.~", rune(c)) || (!markdown && strings.ContainsRune("!'()*", rune(c))) {
			result.WriteByte(c)
		} else {
			result.WriteByte('%')
			result.WriteByte(hex[c>>4])
			result.WriteByte(hex[c&15])
		}
	}
	return result.String()
}
