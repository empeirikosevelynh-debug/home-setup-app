// Package buildinfo holds metadata supplied by the release packager.
package buildinfo

import "fmt"

var Version = "0.1.0-rc.1"
var Commit = "uncommitted"
var Built = "source build"

func String() string {
	return fmt.Sprintf("Golden Gate Setup %s (commit %s; %s; Charm v2)", Version, Commit, Built)
}
