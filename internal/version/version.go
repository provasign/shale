// Package version carries the build version, stamped by -ldflags.
package version

// Version is overridden at build time (see Makefile / goreleaser).
var Version = "dev"
