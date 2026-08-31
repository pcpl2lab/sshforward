# Contributing to sshforward

Thanks for taking the time. Bug reports, small fixes and larger features are all
welcome — this document is the short version of what the project expects.

## Getting set up

Requirements:

- Go 1.27+ (the `go` directive in [`go.mod`](go.mod) is the source of truth; CI
  reads it directly, so bumping it moves CI along with it)
- an OpenSSH client (`ssh`) in `PATH`
- [golangci-lint](https://golangci-lint.run/) **v2.13.2**, built with the same
  Go version as `go.mod` — an older linter refuses to analyse a newer module:

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2
```

```bash
git clone https://github.com/pcpl2/sshforward.git
cd sshforward
go build -o sshforward .        # sshforward.exe on Windows
```

## Before opening a pull request

Run everything CI runs:

```bash
gofmt -l .                      # must print nothing
go vet ./...
go build ./...
go test ./... -count=1
golangci-lint run ./...
```

CI repeats build, vet and tests on Linux, macOS and Windows. Platform-specific
code is the most common source of breakage here, so if you touch anything under
a build tag, say in the PR which platforms you actually tested on.

## Project layout

```
main.go        # entry point, delegates to cmd
cmd/           # CLI commands (cobra): start, stop, list, logs, config, edit, version
internal/
  config/      # YAML config loading and validation (~/.sshforward/config.yaml)
  sshconfig/   # ~/.ssh/config parsing and host validation
  port/        # local port reservation and discovery
  tunnel/      # start/stop of ssh processes, state files, file locking
               # platform-specific files: *_windows.go, *_unix.go
docs/demo/     # asciinema walkthroughs
```

## Conventions

- **Platform code lives in `*_windows.go` / `*_unix.go`** behind build tags, with
  a shared, platform-neutral API in the plain file next to them. Never branch on
  `runtime.GOOS` inside shared code.
- **Validate before `exec.Command`.** Hosts are regex-checked and ports are
  range-checked before they reach the ssh command line. Arguments are passed as
  an argv slice — never build a command string.
- **The system `ssh` is the transport.** The project deliberately shells out to
  the OpenSSH client instead of using a Go SSH library, so that `~/.ssh/config`,
  agents and jump hosts all keep working. Please do not replace it.
- **Errors reach the user.** A command's `RunE` returns errors; `cmd.Execute`
  prints exactly one `Error: …` line. Do not swallow a parser's diagnostics
  behind a generic message.
- **Comments explain why**, not what. If a line encodes a platform quirk or a
  race, say which one.
- **Tests use `t.TempDir()`** for isolation and never touch the real
  `~/.sshforward`. Tests that need a working `ssh` skip themselves:

```go
if _, err := exec.LookPath("ssh"); err != nil {
    t.Skip("ssh not available, skipping integration test")
}
```

- **Failure messages name the inputs**, and read `got X, want Y`.
- **Do not commit binaries.** `sshforward` and `sshforward.exe` are gitignored.

## Commits and pull requests

Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add per-port stop
fix: release the local port reservation before spawning ssh
docs: document reserved port handling
test: cover zombie processes on unix
chore: bump golangci-lint
```

The release changelog is generated from these, and `docs:`, `test:` and
`chore:` commits are filtered out of it.

A good pull request:

- does one thing, and says in the description what problem it solves,
- adds a test that fails without the change,
- updates `README.md` when it changes user-visible behaviour,
- leaves `gofmt`, `go vet`, the tests and the linter clean.

## Releases

Maintainers cut a release by pushing a tag:

```bash
git tag v1.2.3
git push origin v1.2.3
```

That triggers [`.github/workflows/release.yml`](.github/workflows/release.yml),
which runs [GoReleaser](https://goreleaser.com/) to build archives for Linux,
macOS and Windows on `amd64` and `arm64`, publish checksums, and generate the
changelog. Version, commit and build date are injected via `-ldflags`; verify
with `sshforward version`.

## Reporting bugs

Open an issue with the output of `sshforward version`, your OS, a minimal
`config.yaml`, the exact command you ran, and what happened. For a tunnel that
will not stay up, `sshforward logs <host> <service>` holds SSH's own account of
why — but read it before pasting, since it is raw SSH debug output.

Security problems go through [SECURITY.md](SECURITY.md) instead, not the public
issue tracker.
