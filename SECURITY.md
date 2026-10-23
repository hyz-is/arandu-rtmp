# Security policy

## Supported versions

The latest minor release of RTMP receives security fixes. Older minor
releases do not: a published Go module version is immutable, so a fix is a new
release rather than a moved tag.

## Reporting a vulnerability

Report it privately, through GitHub's advisory form:

<https://github.com/hyz-is/arandu-rtmp/security/advisories/new>

Do not open a public issue and do not describe the problem in a pull request.
A report that arrives in public is a report every reader of this repository can
act on before there is a release to upgrade to.

Include what you need to reproduce it: the version, the wiring, and the request
or call that triggers it.

Expect an acknowledgement within a few days. If the report is confirmed, the
fix, the release and the advisory are published together, and you are credited
unless you ask not to be.

## What is in scope

Anything that lets somebody publish or read a stream the installer did not
allow. In particular:

- a publish accepted without `Config.Authorize` returning true for its key;
- a stream readable from an address `Config.Players` refuses, or from outside
  the host when `Players` is left at its default;
- a revoked key that keeps streaming after `Revalidate`;
- a stream key written to a log or an error in clear;
- a publisher or a reader that can exhaust the receiver's memory or goroutines
  past the timeouts it is configured with.

## What is not

- A vulnerability in the framework itself. Report that to the framework.
- An `Authorize` or `Players` that is too permissive in an application that
  wrote it. What this package ships refuses a receiver without `Authorize` and
  reads back to loopback only; what an application opens is its own.
