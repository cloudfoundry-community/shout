package version

import "fmt"

// Set via -ldflags at build time.
var (
	SemVerMajor      = "0"
	SemVerMinor      = "1"
	SemVerPatch      = "0"
	SemVerPrerelease = "dev"
	SemVerBuild      = ""

	BuildDate      = ""
	BuildVcsUrl    = ""
	BuildVcsId     = ""
	BuildVcsIdDate = ""

	Release = "Megaphone"
)

// Version returns the semantic version string (e.g. "0.1.0-dev").
func Version() string {
	v := fmt.Sprintf("%s.%s.%s", SemVerMajor, SemVerMinor, SemVerPatch)
	if SemVerPrerelease != "" {
		v += "-" + SemVerPrerelease
	}
	return v
}

// VersionFull returns the version with build metadata (e.g. "0.1.0-dev+abc1234").
func VersionFull() string {
	v := Version()
	if SemVerBuild != "" {
		v += "+" + SemVerBuild
	}
	return v
}
