# Changelog

All notable changes to sshforward are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `sshforward add` adds a service to the config with an interactive form, and
  `sshforward manage` adds, edits, renames or deletes services the same way.
  Each change is previewed and needs confirmation. Only the affected service
  changes; the rest of the file, comments included, is preserved. Without a
  terminal both commands refuse and point to `sshforward edit`; all other
  commands are unaffected.

## [0.1.1] - 2026-10-26

### Fixed

- Tunnels left over from before a reboot are now always recognised as dead.
  Previously, if the stored PID had been reassigned to another `ssh` process
  (an interactive session, git, an editor's remote helper), the stale tunnel
  showed as `active` in `list`, `start` refused with "already running", and
  `stop` killed that unrelated `ssh` process. sshforward now also checks that
  the process was created before the tunnel was recorded. State files written
  by 0.1.0 are handled too, so nothing needs to be cleaned up by hand.
- `logs` reports a tunnel's status with the same check, instead of showing a
  stale tunnel as `active`.
- Stopping a stale entry whose PID now belongs to a tunnel started after the
  reboot no longer kills that new tunnel.

### Security

- Builds now use Go 1.27.3, pinned with a `toolchain` directive, which fixes
  vulnerabilities in the standard library reachable from sshforward:
  `net/http` and HTTP/2 (GO-2026-6603, -6605, -6608, -6610, -6611, -6613,
  -6617), `crypto/tls` (GO-2026-6607) and `crypto/x509` (GO-2026-6809).
  Previously CI could build releases with any cached Go 1.27.x.

### Changed

- ssh logs are kept for 14 days after their tunnel ends instead of being
  deleted right away (dead tunnels in `list`) or never (failed starts, which
  piled up in `~/.sshforward/tunnels/`). `list` removes the expired ones.
- `logs` also shows logs of runs that have ended — failed starts, dead
  tunnels, earlier runs of a restarted tunnel — so the reason a tunnel failed
  or dropped can still be looked up later.
- Starting a tunnel again archives the previous run's log as
  `<host>-<service>[-<port>].<timestamp>.log` instead of appending to it.
- `stop` keeps the log of a tunnel that had already died on its own; only the
  log of a tunnel it actually stopped is removed.

## [0.1.0] - 2026-09-01

First public release.

[Unreleased]: https://github.com/pcpl2lab/sshforward/compare/v0.1.1...HEAD
[0.1.1]: https://github.com/pcpl2lab/sshforward/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/pcpl2lab/sshforward/releases/tag/v0.1.0
