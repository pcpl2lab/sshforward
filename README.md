# sshforward

[![CI](https://github.com/pcpl2/sshforward/actions/workflows/ci.yml/badge.svg)](https://github.com/pcpl2/sshforward/actions/workflows/ci.yml)
[![License: BSD-2-Clause](https://img.shields.io/badge/license-BSD--2--Clause-blue.svg)](LICENSE)

A cross-platform CLI tool for managing SSH local port forwarding tunnels as background processes.

Define your services once, connect with a single command.

## Demo

Scripted walkthroughs (not live recordings) live in [`docs/demo/`](docs/demo).
Play them locally with [asciinema](https://asciinema.org/):

```bash
asciinema play docs/demo/demo-basic.cast
asciinema play docs/demo/demo-multiport.cast
asciinema play docs/demo/demo-troubleshooting.cast
```

## Features

- **Background tunnels** — start tunnels that persist after the terminal closes
- **Multi-port services** — forward multiple ports per service (e.g. Gitea web + SSH)
- **Automatic port selection** — defaults to matching the remote port, or picks a free
  unprivileged one
- **SSH config integration** — validates hosts against your `~/.ssh/config`
- **Config validation** — port ranges are range-checked and typos like `localport`
  are caught with a suggestion
- **Tunnel monitoring** — list active tunnels, view SSH logs, auto-cleanup dead processes
- **Update checks** — tells you when a newer release exists, and can install it
  itself when you did not use a package manager
- **Cross-platform** — Windows, Linux, macOS

## Requirements

- `ssh` client available in `PATH` (OpenSSH).
- **Non-interactive authentication.** Tunnels run detached with no terminal,
  so SSH cannot prompt. This means:
  - Use key-based auth with a key that has **no passphrase**, or an agent
    (`ssh-agent` / Pageant) already unlocked.
  - The target host must be resolvable via your `~/.ssh/config`.
  - `BatchMode=yes` is enforced — any prompt becomes an immediate, logged
    failure instead of a hung process. Check `sshforward logs <host> <service>`.
  - New host keys are accepted automatically (`accept-new`); a **changed**
    host key still aborts the connection.

## Installation

### Prebuilt binaries

Download a prebuilt binary for your platform from the
[Releases](https://github.com/pcpl2/sshforward/releases) page.
Archives are published for Linux, macOS and Windows on `amd64` and `arm64`,
together with a `checksums.txt`.

### From source

```bash
go install github.com/pcpl2/sshforward@latest
```

### Build manually

```bash
git clone https://github.com/pcpl2/sshforward.git
cd sshforward
go build -o sshforward .        # sshforward.exe on Windows
```

Check the version:

```bash
sshforward version
```

### Prerequisites (building from source)

- Go 1.27+ (see the `go` directive in [`go.mod`](go.mod))
- OpenSSH client (`ssh`) in PATH

## Quick Start

**1. Create a config file** at `~/.sshforward/config.yaml`
(`sshforward edit` creates it for you with a commented template):

```yaml
services:
  mysql:
    remote_port: 3306

  gitea:
    remote_host: 127.0.0.1
    ports:
      - name: web
        remote_port: 3000
      - name: ssh
        remote_port: 2222
```

**2. Start a tunnel:**

```
$ sshforward start myserver gitea
Tunnel myserver/gitea/web started: localhost:3000 -> 127.0.0.1:3000 (PID: 48052)
Tunnel myserver/gitea/ssh started: localhost:2222 -> 127.0.0.1:2222 (PID: 44048)
```

**3. Check status:**

```
$ sshforward list
HOST       SERVICE   PORT   LOCAL PORT   REMOTE           PID     STATUS
myserver   gitea     web    3000         127.0.0.1:3000   48052   active
myserver   gitea     ssh    2222         127.0.0.1:2222   44048   active
```

**4. Stop when done:**

```
$ sshforward stop myserver gitea
Tunnel myserver/gitea stopped
```

## Configuration

Config file location:

| Platform      | Path                                      |
|---------------|-------------------------------------------|
| Linux / macOS | `~/.sshforward/config.yaml`               |
| Windows       | `%USERPROFILE%\.sshforward\config.yaml`   |

The directory is created with `0700` and the config file with `0600`.

### Top-level keys

| Key | Default | Meaning |
|---|---|---|
| `services` | — | The service definitions, described below |
| `update_check` | `true` | Whether to check daily for a newer release |

### Single-port service

```yaml
services:
  mysql:
    remote_host: 127.0.0.1   # optional, defaults to 127.0.0.1
    remote_port: 3306         # required
    local_port: 13306         # optional, defaults to remote_port
```

### Multi-port service

```yaml
services:
  gitea:
    remote_host: 10.0.0.5    # shared default for all ports
    ports:
      - name: web             # required on every entry
        remote_port: 3000
      - name: ssh
        remote_port: 2222
        remote_host: 10.0.0.6  # override per port
        local_port: 12222       # override per port
```

Rules for `ports:`

- At least **two** entries — use `remote_port` directly for a single port.
- Every entry needs a unique `name`; the name becomes part of the state file,
  the log file and the `list` output.
- `remote_port` and `ports` are mutually exclusive within one service.

### Port selection behavior

| Config                   | Behavior                                        |
|--------------------------|-------------------------------------------------|
| *(omitted)*              | Use same port as `remote_port`                  |
| `local_port: 8080`       | Use port 8080                                   |
| `local_port: auto`       | Pick a random free port (always ≥ 1024)         |
| `local_port: 0`          | Pick a random free port (always ≥ 1024)         |

Automatic selection never hands out a port from the reserved system range
(1–1023): those are assigned to standard services and binding them requires
elevated privileges on Unix. An explicit `local_port` below 1024 is still
accepted — you are then responsible for having the rights to bind it.

`remote_port` and `local_port` are validated to be within `1–65535` while the
config is read, so a typo is reported up front instead of surfacing later as a
confusing "port is already in use".

### Validation errors

Unknown keys are rejected with a suggestion:

```
$ sshforward config
Error: service "mysql" line 4: unknown field "localport" (did you mean "local_port"?)
```

```
$ sshforward config
Error: service "mysql": local_port: 99999 is out of range: must be 1-65535
```

## Commands

| Command | Description |
|---------|-------------|
| `sshforward start <host> <service>` | Start tunnel(s) in background |
| `sshforward stop <host> <service>` | Stop all ports for a service |
| `sshforward stop --all` | Stop all active tunnels |
| `sshforward list` | List tunnels with status (auto-cleans dead ones) |
| `sshforward logs [<host> <service>]` | View SSH logs for debugging |
| `sshforward config` | Display loaded configuration |
| `sshforward edit` | Open the config file in `$VISUAL` / `$EDITOR` |
| `sshforward update` | Install the newest release, or say how to update |
| `sshforward update --check` | Report whether a newer release exists, change nothing |
| `sshforward version` | Print version, commit, build date and os/arch |

Notes:

- `stop` always stops **all** ports of a service; there is no per-port stop.
- `list` prints dead tunnels once with status `DEAD`, dumps their SSH log to
  stderr, and then removes their state and log files.
- `logs` without arguments shows the logs of every known tunnel.
- `edit` falls back to `notepad.exe` on Windows and to `nano`, `vim` or `vi`
  elsewhere when neither `$VISUAL` nor `$EDITOR` is set.
- `sshforward --version` is equivalent to `sshforward version` for the version
  string alone.
- Every command exits `0` on success and `1` on failure, printing a single
  `Error: …` line to stderr.

## Updates

### Checking

Once a day, at most, sshforward asks the GitHub API for the newest release and
remembers the answer in `~/.sshforward/update-check.json`. When a newer version
exists, one line goes to **stderr** after the command finishes:

```
A newer sshforward is available: v1.5.0 (you have v1.4.0). Run 'sshforward update'.
```

The check runs in the background and is abandoned if the command finishes first,
so it never delays anything. Every failure is silent: being offline, behind a
proxy or rate-limited is not something an unrelated command should complain
about. Development builds never check, and the notice never touches stdout, so
`list` and `config` stay parsable.

Turn it off permanently in the config:

```yaml
update_check: false
```

or per invocation with `SSHFORWARD_NO_UPDATE_CHECK=1`. `GITHUB_TOKEN`, if set,
is used to raise the anonymous rate limit — useful behind a shared office IP.

### Updating

```bash
sshforward update           # install the newest release
sshforward update --check   # report only, change nothing
```

When a package manager installed sshforward, `update` refuses to touch the
binary and prints that manager's own command instead — overwriting a file
`apt`, `brew`, `scoop` or `winget` owns breaks its manifest and is undone by the
next upgrade:

```
$ sshforward update
A newer release is available: v1.5.0 (you have v1.4.0)

This copy was installed with homebrew. Update it with:
  brew upgrade sshforward
```

Otherwise sshforward downloads the release archive for your platform, **verifies
it against the release's `checksums.txt`**, and replaces its own binary through
renames. A digest that does not match aborts the update with the original binary
untouched. Detection is by path, so it can be wrong; two things keep that safe:
a binary the current user cannot write is never replaced, and
`SSHFORWARD_INSTALL_SOURCE` (`homebrew`, `scoop`, `winget`, `deb`, `manual`)
overrides the guess outright.

## How It Works

1. Reads the service definition from `~/.sshforward/config.yaml`
2. Validates the host exists in `~/.ssh/config` (exact match; wildcard
   patterns such as `Host *` are ignored)
3. Resolves local ports (default = remote port, or finds a free one ≥ 1024)
4. Launches `ssh -N -L <local>:<remote_host>:<remote> <host>` as a detached
   background process
5. Saves tunnel state (PID, ports, timestamps) to `~/.sshforward/tunnels/`
6. Uses file locking to prevent race conditions on concurrent start/stop

### Files on disk

All under `~/.sshforward/` (`%USERPROFILE%\.sshforward\` on Windows):

| Path                                        | Purpose                                 |
|---------------------------------------------|-----------------------------------------|
| `config.yaml`                                | Service definitions                     |
| `tunnels/<host>-<service>[-<port>].json`     | Tunnel state (PID, ports, started at)   |
| `tunnels/<host>-<service>[-<port>].log`      | SSH stderr, written by `ssh -E`         |
| `tunnels/<host>-<service>[-<port>].lock`     | Lock held during start/stop             |
| `update-check.json`                          | Last release lookup, refreshed daily    |

### Process handling

- **Detached**: `CREATE_NEW_PROCESS_GROUP | DETACHED_PROCESS` on Windows,
  `setpgid` on Unix, so the tunnel survives the terminal that started it.
- **Logging**: SSH writes its own log via `-E`, so no file handles are
  inherited — required for a fully detached process on Windows.
- **Liveness**: a stored PID counts as a live tunnel only when the process
  exists *and* its image is `ssh`, so a recycled PID cannot masquerade as a
  running tunnel. An exited-but-unreaped process counts as dead.
- **Port reservation**: a chosen local port is held open by sshforward from the
  moment it is picked until the instant `ssh` is spawned, so a concurrent
  `sshforward` run — or another port of the same service — cannot take it in
  between.
- **Stopping**: `stop` signals the process and waits for it to actually exit,
  escalating to a forced kill after a grace period. The state file is removed
  only once the process is gone, so a successful `stop` means the local port is
  free.
- **Locking**: `LockFileEx` on Windows, `flock` on Unix.

### SSH options applied automatically

- `BatchMode=yes` — never prompt; any prompt becomes a logged failure
- `StrictHostKeyChecking=accept-new` — trust new hosts, reject changed keys
- `ExitOnForwardFailure=yes` — fail fast if the port forward can't be established
- `ConnectTimeout=10` — don't hang on unreachable hosts
- `ServerAliveInterval=15` / `ServerAliveCountMax=3` — detect dead connections

### Input validation

Host names and `remote_host` values are restricted to `[a-zA-Z0-9._-]` and port
numbers to `1–65535` before they reach `exec.Command`. Arguments are passed as
an argv slice, never through a shell.

## Troubleshooting

**Tunnel starts but shows as DEAD in `list`:**

```bash
sshforward logs myserver mysql
```

Common causes:

- Host unreachable — check `ssh myserver` manually
- Port already in use — another process is binding the local port
- Authentication failure — the key needs a passphrase, or the agent is locked;
  remember that `BatchMode=yes` turns any prompt into an immediate failure

**`host "…" not found in …/.ssh/config. Available hosts: …`:**

The host must be defined as a literal `Host` entry. Wildcard patterns are
skipped, so `Host *.example.com` does not make `web.example.com` usable — add an
explicit entry.

**`no config file at …`:**

Run `sshforward edit` to create the file from a commented template.

**`cannot acquire lock (another operation in progress?)`:**

Another `sshforward` process is starting or stopping the same service. If no
such process exists, remove the stale
`~/.sshforward/tunnels/<host>-<service>.lock` file.

## Limitations

- `stop` operates per service, not per port of a multi-port service.
- State files are plain JSON in your home directory and are trusted as written
  by `sshforward` itself; editing them by hand is not supported.
- A local port is held reserved until `ssh` is spawned, but the microseconds
  between handing it over and SSH's own `bind()` cannot be closed from the
  outside. If another process wins that race, `ExitOnForwardFailure=yes` makes
  SSH fail immediately and `start` reports it — retry.

## Development

```bash
go generate ./...               # Windows version resource (see winres/)
go build -o sshforward .        # build
go test ./... -count=1          # tests (SSH-dependent tests skip without ssh)
go vet ./...                    # static analysis
golangci-lint run ./...         # lint (v2.13.2, see .golangci.yml)
```

On Windows the binary carries a full version resource — product name,
description, company, copyright and version — plus an icon and an application
manifest declaring `asInvoker` (so it never triggers a UAC prompt) and long-path
awareness. It is built from `winres/winres.json`; release builds get the real
tag, local builds are stamped `0.0.0.0`.

CI runs build, vet and tests on Linux, macOS and Windows, plus a lint job; see
[`.github/workflows/ci.yml`](.github/workflows/ci.yml). The Go version is taken
from `go.mod`, so bumping the `go` directive is enough to move CI along with it.

Releases are cut by pushing a `v*` tag, which triggers
[GoReleaser](https://goreleaser.com/) via
[`.github/workflows/release.yml`](.github/workflows/release.yml). Version,
commit and build date are injected into the binary through `-ldflags`.

## Contributing

Setup, conventions and the pull request checklist live in
[CONTRIBUTING.md](CONTRIBUTING.md).

## Security

Please report vulnerabilities privately — see [SECURITY.md](SECURITY.md), which
also documents the security properties sshforward relies on.

## License

BSD 2-Clause License — see [LICENSE](LICENSE).
