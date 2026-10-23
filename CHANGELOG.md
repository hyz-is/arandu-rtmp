# Changelog

Everything worth knowing about a release of RTMP is recorded here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and
the versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

A published module version is immutable: Go serves it from the proxy forever, so
a release is corrected by another release and never by moving a tag.

## [Unreleased]

### Added

- The RTMP receiver: `New(Config)`, `Start`, `Serve`, `Close`, `Streams` and
  `Revalidate`. Publishers are authorized by stream key through
  `Config.Authorize`; readers are loopback only unless `Config.Players` says
  otherwise; a late reader starts at a key frame.

