---
name: rtmp-module
description: Change the RTMP receiver itself. Use when the request is to "support another codec", "add audio", "add a config option", "change a default", "report another stat", "let another host read", "drop a stream when", "the relay stutters", "a late reader gets a grey picture", "add a test for the relay", or when a change touches module.go, config.go or relay.go. Covers what the package is and refuses to become, where each concern lives, the lifecycle the framework drives, the locking rule, and how the suite proves the relay.
license: MIT
---

# Changing the receiver

The package is a relay: publishers in, local readers out, state in memory.
It has no table, no route, no policy and no service, and none of them belongs
here. Who owns a key, which tenant a stream belongs to, who may see the stats:
those are the installer's, answered through `Config.Authorize`, `Config.Players`
and whatever the application does with `Streams()`.

## Where things live

| file | what it holds |
| --- | --- |
| `config.go` | `Config`, the defaults as exported constants, `Loopback`, `ErrNoAuthorizer` |
| `module.go` | `Module`: `New`, `Name`, the empty `Routes`, `Start`, `Serve`, `Addr`, `Close`, `Streams`, `Revalidate` |
| `relay.go` | one connection's life: the stream key, `publish`, `play`, the per-stream fan-out and `Stats` |

A new setting is a field on `Config` with its default as a named constant read
by `withDefaults`, never a literal buried in the relay. A setting without a safe
default does not get one: it is required, the way `Authorize` is.

## The lifecycle

`New` validates and listens on nothing. `Start` binds `Addr` and fails at boot
when it cannot; `Serve` takes any listener, which is what the tests use. `Close`
stops the listener, stops every stream and waits for every connection
goroutine. A goroutine started anywhere else has to end on the context or on
`Close`, or `Close` returns while it still runs.

## Locking

`Module.mu` guards the stream map and the listener and nothing slow: copy what
you need under the lock, release it, then call `Authorize`, write to a
connection or stop a stream. `Authorize` is the installer's code and may hit a
database; holding the lock across it stalls every publisher.

## What the relay promises

- A reader that joins late waits for a key frame, so the first picture it
  decodes is whole.
- A newer publisher of a key replaces the older one.
- A revoked key stops on `Revalidate`, not on its next reconnect.
- A silent publisher is dropped after `IdleTimeout`.
- Keys are logged redacted: they are credentials.

A change that weakens one of these is a breaking change and goes in
`CHANGELOG.md` and `UPGRADE.md`, even when no signature moves.

## Proving it

`tests/Feature/relay_test.go` publishes and reads real RTMP over loopback;
`relay_internal_test.go` covers the pieces that need unexported access. A
behaviour without a test that fails when it breaks is not done. Run the four
gates in `rtmp-release` with `GOWORK=off` before anything leaves your machine.
