// Package rules parses and validates the formats only Trail Guide itself reads, such as project configuration,
// change notes, Git references, and Markdown links, and renders the rules and groups that authors create, without
// accessing the filesystem. Rule files, group metadata, release records, and the library section of a configuration
// belong to the public trailguide package, which other tools also use.
package rules
