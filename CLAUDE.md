# sshforward

CLI tool for managing SSH local port forwarding tunnels as background processes.

## Build & Test

```bash
go build -o sshforward.exe .   # build (Windows)
go build -o sshforward .       # build (Linux/macOS)
go test ./... -count=1          # run all tests
go vet ./...                    # static analysis
```

## Project Structure

```
cmd/           # CLI commands (cobra): start, stop, list, config, logs
internal/
  config/      # YAML config loading (~/.sshforward/config.yaml)
  sshconfig/   # SSH config parsing (~/.ssh/config host validation)
  port/        # Free port discovery (net.Listen on :0)
  tunnel/      # Core logic: start/stop SSH processes, state files, file locking
               # Platform-specific files: *_windows.go, *_unix.go
```

## Architecture Decisions

- **Systemowy ssh** — aplikacja wywołuje `ssh` z PATH, nie używa wbudowanej biblioteki Go. Pełna kompatybilność z `~/.ssh/config`.
- **Detached process** — na Windows: `CREATE_NEW_PROCESS_GROUP | DETACHED_PROCESS`. Na Unix: `Setpgid: true`.
- **SSH -E logfile** — SSH sam loguje do pliku (zero dziedziczenia file handleów). Krytyczne na Windows z DETACHED_PROCESS.
- **State files** — JSON w `~/.sshforward/tunnels/`. Każdy port = osobny plik (np. `p1-gitea-web.json`).
- **File locking** — `LockFileEx` na Windows, `flock` na Unix. Zapobiega race conditions przy start/stop.
- **local_port default** — brak `local_port` = ten sam co `remote_port`. Explicit `auto` lub `0` = losowy wolny port.
- **Multi-port services** — serwis może mieć wiele portów (np. gitea: web+ssh). `ports:` array z wymaganym `name`.
- **Unknown field detection** — config parser wykrywa literówki (np. `localport` → "did you mean `local_port`?").

## Config Format (~/.sshforward/config.yaml)

```yaml
services:
  mysql:
    remote_port: 3306           # local_port defaults to 3306
  gitea:
    remote_host: 127.0.0.1
    ports:
      - name: web
        remote_port: 3000       # local_port defaults to 3000
      - name: ssh
        remote_port: 2222
        local_port: 2222        # explicit
```

## Usage

```bash
sshforward start <host> <service>   # start tunnel(s)
sshforward stop <host> <service>    # stop all ports for service
sshforward stop --all               # stop everything
sshforward list                     # show active tunnels + dead cleanup
sshforward logs <host> <service>    # show SSH logs
sshforward config                   # show loaded config
```

## Conventions

- Do not commit automatically — only when explicitly asked.
- Platform-specific code goes in `*_windows.go` / `*_unix.go` with build tags.
- Input validation before `exec.Command` — regex-checked host, range-checked ports.
- Tests use `t.TempDir()` for isolation. SSH integration tests skip with `exec.LookPath("ssh")`.
