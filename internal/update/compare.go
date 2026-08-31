package update

import (
	"strings"

	"golang.org/x/mod/semver"
)

// IsNewer reports whether latest is a release worth upgrading to from current.
//
// A version that does not parse — a locally built binary reporting "dev", or a
// tag that is not semantic — is never treated as an update: what we cannot
// compare, we do not nag about. Prereleases are offered only to a binary that
// is itself a prerelease, so a stable install never drifts onto a release
// candidate by accident.
func IsNewer(current, latest string) bool {
	cur, lat := normalize(current), normalize(latest)
	if !semver.IsValid(cur) || !semver.IsValid(lat) {
		return false
	}
	if semver.Prerelease(lat) != "" && semver.Prerelease(cur) == "" {
		return false
	}
	return semver.Compare(lat, cur) > 0
}

// normalize gives a version the "v" prefix that semver requires.
func normalize(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || strings.HasPrefix(v, "v") {
		return v
	}
	return "v" + v
}
