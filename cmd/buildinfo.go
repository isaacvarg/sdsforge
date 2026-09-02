package cmd

import (
	"runtime/debug"
	"strings"
)

// Build information about sdsforge itself, stamped in at release time with
// -ldflags -X. Kept apart from version.go, which is about the versions of the
// documents sdsforge produces rather than the version of sdsforge.
var (
	buildVersion = ""
	buildCommit  = "none"
	buildDate    = "unknown"
)

// resolveVersion reports the version this binary was built as.
//
// A release sets buildVersion directly. Failing that, a binary from 'go install
// <module>@<version>' carries its module version in build info, which is just
// as true a version and worth reporting. Anything else was built from a working
// tree and says so, rather than claiming a release it is not.
func resolveVersion() string {
	if buildVersion != "" {
		return buildVersion
	}

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}

	// Go stamps Main.Version even for a plain 'go build', deriving a
	// pseudo-version from the checkout. That is not a version anyone can
	// install, so it must not be reported as one. What separates the two is
	// where the source came from: 'go install <module>@<version>' builds from
	// the module cache and records no vcs settings, while a build from a
	// working tree records them.
	if rev, dirty, fromVCS := vcsInfo(info); fromVCS {
		return localVersion(rev, dirty)
	}
	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}

// vcsInfo pulls out what the build recorded about the checkout it came from.
func vcsInfo(info *debug.BuildInfo) (revision string, dirty, ok bool) {
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision, ok = setting.Value, true
		case "vcs.modified":
			dirty = setting.Value == "true"
		}
	}
	return revision, dirty, ok
}

// localVersion names a build from a working tree by the commit it sits on, so a
// bug report from an unreleased binary still says which code ran.
func localVersion(revision string, dirty bool) string {
	if revision == "" {
		return "dev"
	}
	short := revision
	if len(short) > 7 {
		short = short[:7]
	}
	if dirty {
		short += "-dirty"
	}
	return "dev (" + short + ")"
}

// versionLine is what --version prints. A release adds the commit and build
// date, so a bug report can name exactly what was run; a dev build already
// carries its commit in the version itself.
func versionLine() string {
	var b strings.Builder
	b.WriteString("sdsforge ")
	b.WriteString(resolveVersion())
	if buildVersion != "" {
		b.WriteString(" (" + buildCommit + ", built " + buildDate + ")")
	}
	b.WriteString("\n")
	return b.String()
}
