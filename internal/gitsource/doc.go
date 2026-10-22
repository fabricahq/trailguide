// Package gitsource validates the Git repository addresses and refs that configurations and library manifests
// declare, without fetching anything. Both Trail Guide's configuration parser and the public trailguide package read
// them through it, so a project and a library's consumers accept the same sources.
package gitsource
