---
name: rtmp-package
description: Install, wire and use the RTMP receiver (Go, Arandu) in an application. Use when the request is to "install rtmp", "go get github.com/hyz-is/arandu-rtmp", "take cameras that push RTMP", "a camera behind 4G or carrier NAT", "wire the receiver into bootstrap", "the camera connects and is dropped", "publish refused", "ffmpeg cannot read rtmp://127.0.0.1", "a revoked key keeps streaming", "address already in use on 1935", "show which cameras are pushing", or when a project's go.mod already requires github.com/hyz-is/arandu-rtmp. Covers what the receiver is and is not, the one required setting, where the wiring goes, who may read a stream, revoking keys, the stats it reports and the port it needs.
license: MIT
metadata:
  audience: app
---

# Using RTMP

A relay, not a media server. A camera behind carrier NAT or a 4G modem cannot
be reached by a server that pulls RTSP, but it can open a connection outwards:
it publishes to `rtmp://host/live/<key>`, the receiver asks the application
whether that key may publish, and a process on the same host (an engine running
ffmpeg) reads `rtmp://127.0.0.1/live/<key>` back. H.264 and H.265 video are
forwarded as they arrive; a reader that joins late starts at the next key frame.

It records nothing, transcodes nothing, owns no table and registers no route.
Its state lives in memory for as long as each connection does.

## Install

```bash
go get github.com/hyz-is/arandu-rtmp@v0.1.1
```

No migration: the manifest declares `migrations = false`. It declares
`network = true`, because it listens on a port of its own.

## Wire it

By hand, in `bootstrap/app.go` or the provider that builds the application's
camera side, like every Arandu module. `Authorize` is the only required field,
and `New` refuses a `Config` without it (`rtmp.ErrNoAuthorizer`): a receiver
anybody could publish to is not a configuration this package offers.

```go
import rtmp "github.com/hyz-is/arandu-rtmp"

receiver, err := rtmp.New(rtmp.Config{
	Addr: cfg.RTMPAddr, // ":1935" when empty
	Authorize: func(ctx context.Context, key string) bool {
		return cameras.HasIngestKey(ctx, key) // the application's own lookup
	},
	RevalidateEvery: time.Minute, // optional: drop revoked keys on a timer
})
if err != nil {
	return err
}
app.Register(receiver)
```

`*rtmp.Module` is a `foundation.Module` (`Name`, an empty `Routes`) with
`Start(ctx)` and `Close(ctx)`, so the framework starts it with the server and
closes it on shutdown. `Start` binds the port at boot and returns the error when
it cannot, rather than leaving a receiver that silently is not there.

The key is the stream's name and its credential at once. Where keys live, which
tenant a key belongs to and which camera it feeds is the application's: keep
that lookup in a service behind `Authorize`, and read the tenant there from the
application's own records, never from anything the publisher sent.

## Optional settings

| field | default | what it does |
| --- | --- | --- |
| `Addr` | `:1935` | TCP address to listen on |
| `App` | `live` | the application name in `rtmp://host/<app>/<key>` |
| `Players` | `rtmp.Loopback` | who may read a stream back, by remote address |
| `IdleTimeout` | 10s | a publisher that sends nothing this long is dropped |
| `HandshakeTimeout` | 10s | bounds the handshake and the connect/publish commands |
| `RevalidateEvery` | 0 (off) | asks `Authorize` again about every live stream at this interval |
| `Logger` | `slog.Default()` | one line per publisher that connects, is refused or leaves; keys are redacted |

## Who may read

Only loopback, by default: the stream is meant for the engine beside the
receiver. Widening `Players` puts camera video on the network; do it only for a
private address range the application controls, never for the internet.

## Revoking a key

When the set of keys changes (a camera removed, a key replaced), call
`receiver.Revalidate(ctx)`; it asks `Authorize` again about every stream being
published, drops the ones it no longer allows, and returns how many it dropped.
`RevalidateEvery` does the same on a timer, for keys revoked somewhere the
application does not hear about. A newer publisher of the same key replaces the
older one.

## What it reports

`receiver.Streams()` returns a `[]rtmp.Stats`, ordered by key: the key, the
publisher's address, the codec (`H264` or `H265`), width and height from the
parameter sets, frames per second over the last five seconds, when it connected
and when its last frame arrived, bytes received and how many readers it has.
Render it from the application's own controller, behind the application's own
Policy; the package has no route to do it for you.

## Deploying

The port is TCP and has to be published by the container and opened in the
firewall. Readers connect to `127.0.0.1`, so the engine that reads the streams
runs in the same container or the same network namespace as the receiver.

## When something goes wrong

| symptom | cause |
| --- | --- |
| the camera connects and is dropped at once | `Authorize` returned false for its key, or it published to an application name other than `App` |
| `address already in use` at boot | another process holds `Addr`; `Start` refuses rather than running without it |
| ffmpeg on another host cannot read | `Players` is loopback only, on purpose |
| a stream stops after ten seconds of a frozen camera | `IdleTimeout`: a camera that lost its uplink usually leaves the TCP connection open |
| width and height are zero | the codec did not carry its parameter sets in a form the receiver reads; the video is still relayed |
