# Changelog

Everything worth knowing about a release of RTMP is recorded here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and
the versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

A published module version is immutable: Go serves it from the proxy forever, so
a release is corrected by another release and never by moving a tag.

## [Unreleased]

## [0.1.1] - 2026-10-23

### Fixed

- The `rtmp-package` skill, which `aru skills:sync` copies into an installing
  application, described the template's table, policy and migration instead of
  the receiver. It now covers `Authorize`, the wiring, the settings, loopback
  readers, `Revalidate`, `Streams` and the port. The maintainer skills,
  `AGENTS.md`, `CONTRIBUTING.md` and `SECURITY.md` say the same.

## [0.1.0] - 2026-10-23

### Added

- The RTMP receiver: `New(Config)`, `Start`, `Serve`, `Close`, `Streams` and
  `Revalidate`, with `Config.RevalidateEvery` for a timer. Publishers are authorized by stream key through
  `Config.Authorize`; readers are loopback only unless `Config.Players` says
  otherwise; a late reader starts at a key frame.

