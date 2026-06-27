# sshforward

A cross-platform CLI tool for managing SSH local port forwarding tunnels as background processes.

Define your services once, connect with a single command.

## Demo

### Basic Usage
<!-- To view locally: asciinema play docs/demo/demo-basic.cast -->
<a href="https://asciinema.org/a/TODO-basic"><img src="https://asciinema.org/a/TODO-basic.svg" width="700"/></a>

### Multi-Port Services
<!-- To view locally: asciinema play docs/demo/demo-multiport.cast -->
<a href="https://asciinema.org/a/TODO-multiport"><img src="https://asciinema.org/a/TODO-multiport.svg" width="700"/></a>

### Error Handling & Troubleshooting
<!-- To view locally: asciinema play docs/demo/demo-troubleshooting.cast -->
<a href="https://asciinema.org/a/TODO-troubleshooting"><img src="https://asciinema.org/a/TODO-troubleshooting.svg" width="700"/></a>

> **Note:** Replace `TODO-*` links above after uploading casts with `asciinema upload docs/demo/<file>.cast`

## Features

- **Background tunnels** — start tunnels that persist after the terminal closes
- **Multi-port services** — forward multiple ports per service (e.g. Gitea web + SSH)
- **Automatic port selection** — defaults to matching the remote port, or picks a free one
- **SSH config integration** — validates hosts against your `~/.ssh/config`
- **Typo detection** — catches config mistakes like `localport` instead of `local_port`
- **Tunnel monitoring** — list active tunnels, view SSH logs, auto-cleanup dead processes
- **Cross-platform** — Windows, Linux, macOS

## Installation

### From source

```bash
go install github.com/pcpl2/sshforward@latest
```

### Build manually

```bash
git clone https://github.com/pcpl2/sshforward.git
cd sshforward
go build -o sshforward .
```

### Prerequisites

- Go 1.21+
- OpenSSH client (`ssh`) in PATH

## Quick Start

**1. Create a config file** at `~/.sshforward/config.yaml`:

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

Config file location: `~/.sshforward/config.yaml`

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
      - name: web             # required for multi-port
        remote_port: 3000
      - name: ssh
        remote_port: 2222
        remote_host: 10.0.0.6  # override per port
        local_port: 12222       # override per port
```

### Port selection behavior

| Config                   | Behavior                              |
|--------------------------|---------------------------------------|
| *(omitted)*              | Use same port as `remote_port`        |
| `local_port: 8080`      | Use port 8080                         |
| `local_port: auto`       | Pick a random free port               |
| `local_port: 0`          | Pick a random free port               |

## Commands

| Command | Description |
|---------|-------------|
| `sshforward start <host> <service>` | Start tunnel(s) in background |
| `sshforward stop <host> <service>` | Stop all ports for a service |
| `sshforward stop --all` | Stop all active tunnels |
| `sshforward list` | List tunnels with status (auto-cleans dead ones) |
| `sshforward logs [<host> <service>]` | View SSH logs for debugging |
| `sshforward config` | Display loaded configuration |

## How It Works

1. Reads service definition from `~/.sshforward/config.yaml`
2. Validates the host exists in `~/.ssh/config`
3. Resolves local ports (default = remote port, or finds a free one)
4. Launches `ssh -N -L <local>:<remote_host>:<remote> <host>` as a detached background process
5. Saves tunnel state (PID, ports, timestamps) to `~/.sshforward/tunnels/`
6. Uses file locking to prevent race conditions on concurrent start/stop

SSH options applied automatically:
- `ExitOnForwardFailure=yes` — fail fast if the port forward can't be established
- `ConnectTimeout=10` — don't hang on unreachable hosts
- `ServerAliveInterval=15` / `ServerAliveCountMax=3` — detect dead connections

## Troubleshooting

**Tunnel starts but shows as DEAD in `list`:**

```bash
sshforward logs myserver mysql
```

Common causes:
- Host unreachable — check `ssh myserver` manually
- Port already in use — another process is binding the local port
- Authentication failure — ensure SSH keys are configured

**Config error on typo:**

```
$ sshforward start myserver mysql
Error: service "mysql" line 3: unknown field "localport" (did you mean "local_port"?)
```

## License

BSD 2-Clause License

