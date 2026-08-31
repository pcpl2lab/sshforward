package main

import "github.com/pcpl2/sshforward/cmd"

// Windows version resource and icon. The generated rsrc_windows_*.syso files
// are not committed: `go generate ./...` produces them for a local build, and
// the release pipeline regenerates them with the real tag. The metadata itself
// lives in winres/winres.json.
//
// The version flags are not optional. go-winres writes the string table only
// when both are set, so omitting them yields a binary whose Properties dialog
// is entirely blank. 0.0.0.0 marks a build that is not a release.
//
//go:generate go run github.com/tc-hib/go-winres@v0.3.3 make --arch amd64,arm64 --file-version 0.0.0.0 --product-version 0.0.0.0

func main() {
	cmd.Execute()
}
