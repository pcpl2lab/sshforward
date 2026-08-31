# Security Policy

## Supported versions

Only the latest released version is supported. Fixes are shipped as a new
release rather than backported.

| Version        | Supported |
|----------------|-----------|
| latest release | ✅        |
| anything older | ❌        |

## Reporting a vulnerability

**Please do not open a public issue for security problems.**

Report privately through GitHub:
[**Report a vulnerability**](https://github.com/pcpl2/sshforward/security/advisories/new)
(Security → Advisories → Report a vulnerability).

Useful things to include:

- affected version (`sshforward version`) and operating system,
- a minimal `config.yaml` or command sequence that reproduces the issue,
- what an attacker gains, and what access they need to begin with.

You can expect an acknowledgement within a few days and an assessment shortly
after. If a fix is warranted, the advisory is published together with the
release that contains it, crediting you unless you ask otherwise.

## Scope

In scope:

- injection of attacker-controlled data into the `ssh` command line,
- escaping the validation applied to hosts, remote hosts and port numbers,
- permissions of files created under `~/.sshforward/`,
- secrets leaking into state files, log files or terminal output,
- weakening of the SSH options sshforward enforces (see below),
- anything letting an unverified binary be installed by `sshforward update`.

Out of scope:

- vulnerabilities in OpenSSH itself — report those to the OpenSSH project,
- consequences of a user's own `~/.ssh/config`, keys or host-key decisions,
- the deliberate requirement for non-interactive authentication (a passphrase
  free key or an unlocked agent); this is documented behaviour, not a defect,
- anything requiring an attacker who can already write to the user's home
  directory or run code as that user.

## Security design notes

Facts worth knowing before reporting, and worth preserving in any patch:

- **No shell.** `ssh` is executed with an argv slice via `exec.Command`; no
  command string is ever handed to a shell.
- **Input validation before exec.** Host names and `remote_host` values must
  match `^[a-zA-Z0-9._-]+$`, and port numbers must be within `1–65535`, checked
  both while reading the config and again immediately before exec.
- **`BatchMode=yes`.** Tunnels are detached and have no terminal, so any prompt
  would hang forever. Batch mode turns prompts into logged failures instead.
- **`StrictHostKeyChecking=accept-new`.** New hosts are trusted on first use; a
  *changed* host key still aborts the connection. This is a deliberate trade-off
  for unattended tunnels — set your own policy in `~/.ssh/config` if you need
  strict checking.
- **File permissions.** `~/.sshforward/` is created with `0700` and files within
  it with `0600`.
- **No credentials handled.** sshforward never reads, stores or forwards keys,
  passphrases or passwords; all authentication is delegated to `ssh` and the
  user's agent.
- **Network egress.** sshforward makes exactly one kind of outbound request of
  its own: a release lookup against `api.github.com`, at most once a day, plus
  the release downloads that `sshforward update` performs. Nothing about the
  user, their hosts or their config is sent. Disable it entirely with
  `update_check: false` or `SSHFORWARD_NO_UPDATE_CHECK=1`.
- **Verified self-update.** `sshforward update` computes the SHA-256 of the
  downloaded archive and compares it against the release's `checksums.txt`
  before anything on disk is touched. A mismatch aborts with the running binary
  untouched. A binary owned by a package manager, or one the current user
  cannot write, is never replaced.
- **Logs.** SSH writes its own log through `-E` to
  `~/.sshforward/tunnels/*.log`. Those logs are shown verbatim by `sshforward
  logs` and by `sshforward list` for dead tunnels — treat them as you would any
  SSH debug output before pasting them into an issue.
