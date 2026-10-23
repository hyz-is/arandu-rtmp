# Working on RTMP

This is an Arandu package: an RTMP receiver. Cameras publish to it, a process on
the same host reads each stream back, and its state lives in memory. It has no
table, no route, no policy and no service; the installer decides which keys may
publish through `Config.Authorize`.
It is a Go module somebody `go get`s and registers by hand in their own
`bootstrap/app.go`, which is the whole difference from working in an
application. There is no service provider, no container and no discovery — if a
line of wiring is not written in the installer's repository, it does not happen.

Read `.agents/skills/` before writing code. Each skill is a procedure, and the
one you need is named by the situation you are in.

## The gates

Nothing is finished until all four exit zero.

```sh
export GOWORK=off
gofmt -l $(find . -name '*.go' -not -path '*/testdata/*' -not -name '*.kyse.go')
go build ./...
go vet ./...
go test -race ./...
```

`GOWORK=off` is not borrowed from somewhere else, and here it is not a
preference either. This checkout may sit beside a Go workspace that lists the
framework repositories and does not list this one; when it does, every command
above fails before it compiles anything:

```
pattern ./...: directory prefix . does not contain modules listed in go.work
or their selected dependencies
```

With the workspace off, the module resolves the framework version in `go.mod` —
which is what CI compiles against, and what somebody's `go get` will get.

Both filters on `gofmt` are load-bearing in the toolchain even where this
repository has nothing for them to skip: `gofmt` is the only tool in the chain
that ignores build tags, and `testdata/` is where a fixture is allowed to be
invalid on purpose.

`aru doctor` is not one of the gates, and running it here costs a minute and
answers nothing:

```
this is not an Arandu project: no go.mod, main.go and arandu.toml together.
Run it from inside a project, or create one with `aru new`
```

It exits 1. It reads applications, and this is a library.

It does not read this one after it is installed either: the doctor walks the
application's own tree and never loads a dependency. Whatever this package must
prove about itself it proves in its own suite.

## What this repository holds

| | measured with |
| --- | --- |
| 3 Go files, one per role, all in one package at the root | `grep -l '^package rtmp' *.go` |
| 3 test files | `find . -name '*_test.go'` |
| 0 routes, 0 tables | `Routes` is empty; `arandu.mod.toml` says `migrations = false` |
| 2 direct dependencies beside the framework, both MIT | `go list -m -f '{{if and (not .Indirect) (not .Main)}}{{.Path}} {{.Version}}{{end}}' all` |

```
config.go   what the application passes in, and the defaults as constants
module.go   New, the lifecycle the framework drives, Streams and Revalidate
relay.go    one connection's life: the key, publish, play, the fan-out, Stats
```

## What does not exist here

| A model reaches for | What is here instead |
| --- | --- |
| a service provider, a container, a `Register()` that discovers things | `New(cfg)`, called by hand in the installer's `bootstrap/app.go` |
| a table of stream keys, a policy, a route to list streams | the installer's: `Config.Authorize` answers for keys, `Streams()` is rendered behind the installer's own Policy |
| recording, transcoding, HLS, players on the internet | nothing. It is a relay; readers are loopback unless `Config.Players` says otherwise |
| an `interface{}` config, a map of options, an env var read at call time | the typed `Config` struct, validated by `New` |
| a `panic` on bad wiring | an `error` from `New` or `Start`. A wiring mistake found at boot costs one restart |
| another dependency | an argument, first. This module is imported into other people's builds |

## What a change has to keep true

1. **Nothing publishes without `Authorize`.** `New` refuses a `Config` without it,
   and every publish asks it.
2. **Readers are loopback by default.** Widening who may read is the installer's
   decision, through `Players`.
3. **A revoked key stops on `Revalidate`**, not on its next reconnect.
4. **Keys are credentials**: logged redacted, never in an error returned to a
   publisher.
5. **`arandu.mod.toml` matches the code.** It declares `network = true` and
   nothing else; a file write, a process or a table is declared in the same
   commit.

`tests/Feature/relay_test.go` publishes and reads real RTMP over loopback and
holds the first three.

## Writing code

Everything in the source is in English: identifiers, doc comments, internal
comments, error messages, log messages, and the names and messages of tests.
`pkg.go.dev` publishes the doc comments and its readers are users of this
package.

Every exported symbol carries a doc comment, and the comment documents the
symbol and nothing else. Why a signature is what it is belongs there when it is
a fact about the code — *"the value is held because Go does not build a type
from a string"* stays. A date, an issue number, a version in progress or the
name of another repository does not.

Tests go under `tests/`, in a capitalized category directory declaring a
lowercase external package: `tests/Unit` holds `package unit_test`,
`tests/Feature` holds `package feature_test`. Both import the package by its
module path, which is what makes them see exactly what a caller sees. A test
that genuinely needs something unexported goes beside the code as
`*_internal_test.go`, and the suffix is how it says so.
