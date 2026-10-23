---
name: rtmp-release
description: Get this Arandu package green, declared and published. Use when the request is to "run the gates", "check it builds", "the tests fail", "go build says the directory prefix does not contain modules listed in go.work", "aru doctor says this is not an Arandu project", "update arandu.mod.toml", "add a dependency", "bump the framework", "tag a release", "publish it", "go get cannot find the module", "write the changelog", or before opening a pull request. Covers the four gates and why each filter is there, what the manifest declares, the immutability of a published module version, and what breaks somebody else's build.
license: MIT
---

# Getting it green and out

## The four gates

Nothing is finished until all four exit zero.

```sh
export GOWORK=off
gofmt -l $(find . -name '*.go' -not -path '*/testdata/*' -not -name '*.kyse.go')
go build ./...
go vet ./...
go test -race ./...
```

`GOWORK=off` first, and here it is not a habit. This checkout may sit beside a
Go workspace that lists the framework repositories and does not list this one.
When it does, `go build ./...` exits 1 before compiling anything:

```
pattern ./...: directory prefix . does not contain modules listed in go.work
or their selected dependencies
```

That is the workspace refusing to answer for a module it was not told about, and
it says nothing about the code. With the workspace off, the module resolves the
versions in `go.mod` — which is what CI compiles against, and what somebody's
`go get` will get. A pass obtained with a workspace active is a pass against
whatever happens to be checked out beside you.

`aru doctor` is not a gate. It exits 1 here with *"this is not an Arandu
project: no go.mod, main.go and arandu.toml together"* — it reads applications,
and this is a library.

CI runs the same four, plus govulncheck and an API diff against the last
release, and the release workflow runs them again on the tagged archive.

## What the manifest declares

`arandu.mod.toml` is a promise to whoever installs the package, and an
application's `aru doctor` compares it with what that application allows.

```toml
name = "hyz-is/rtmp"
framework = ">= 0.56"
profiles = ["conventional", "performance"]

[permissions]
network = true
filesystem = false
exec = false
migrations = false
```

`network = true` because the receiver listens on a port of its own. The others
are false and stay false unless the code starts doing it: a call that touches the
disk, runs a program or needs a table is a line changed here in the same commit.

The `framework` line is a floor, and it is a second declaration of something
`go.mod` already says. `tests/Unit/release_test.go` fails when the two disagree,
and when this skill stops teaching the floor the manifest declares. Move all
three together when you bump, and bump only when the code needs it: an installer
on an older framework should not have to upgrade to take a fix.

## Adding a dependency

Not without an argument. This module goes into other people's builds: every
`require` is a download and an audit surface for every project that installs
it. The RTMP protocol comes from `github.com/bluenviron/gortmplib` and the codec
parsing from `github.com/bluenviron/mediacommon/v2`, both MIT; anything more
goes with a note in the pull request saying what it does that these and the
framework do not.

```sh
export GOWORK=off
go list -m -f '{{if and (not .Indirect) (not .Main)}}{{.Path}} {{.Version}}{{end}}' all
```

## Publishing

A published Go module version is **immutable**. The proxy serves it forever, and
a mistake is corrected by another release, never by moving a tag.

Before tagging:

- the four gates pass with `GOWORK=off`;
- `arandu.mod.toml` matches the code;
- `CHANGELOG.md` has the entry under `## [x.y.z] - YYYY-MM-DD`, in the Keep a
  Changelog sections — `Added`, `Changed`, `Fixed`, `Removed` — and nothing
  shipped is left under `[Unreleased]`;
- `UPGRADE.md` has a `## vx.y.z` section, even when it says there is nothing to
  change.

Then push an annotated tag `vx.y.z` on `main`. The release workflow checks the
archive of exactly that tag and publishes the GitHub release from the changelog
entry; a tag without the entry fails there.

While the version starts with `v0.`, breaking is allowed. Breaking *quietly* is
not. The CI step `api diff against the last release` fails unless `UPGRADE.md`
names every removal. Three changes break an installer without touching a
signature, and each one is worth a line in the changelog:

- **A default.** `DefaultAddr`, `DefaultApp` and the timeouts are what an
  application gets by leaving the field empty.
- **Who may read.** Anything that widens `Loopback` puts camera video somewhere
  it was not.
- **The URL shape.** Cameras are configured by hand with
  `rtmp://host/<app>/<key>`; a change there is a site visit per camera.

## Reporting a vulnerability

Not in an issue and not in a pull request. `SECURITY.md` has the private
advisory address. A publish accepted without `Authorize` saying yes, a stream
readable from outside `Players`, a key written to a log unredacted, or a revoked
key that keeps streaming after `Revalidate` are all in scope.
