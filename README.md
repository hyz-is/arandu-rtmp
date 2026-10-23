<p align="center">
  <img src=".github/logo.svg" alt="Arandu" width="180">
</p>

# Arandu RTMP

An RTMP receiver for Arandu applications. Cameras, encoders and recorders
publish to it; a process on the same host reads each stream back.

A camera behind carrier NAT or a 4G modem cannot be reached by a server that
pulls RTSP, but it can open a connection outwards. It publishes to
`rtmp://host/live/<key>`; the receiver asks the application whether that key
may publish; the application's own reader (an engine running ffmpeg) plays
`rtmp://127.0.0.1/live/<key>` back. The key is the stream's name and its
credential at once, and the application decides which keys exist.

It is a relay, not a media server: no recording, no transcoding, no players on
the internet. H.264 and H.265 video are forwarded as they arrive; a reader that
joins late starts at the next key frame.

## Install

```bash
go get github.com/hyz-is/arandu-rtmp
```

## Wire it

The module is registered by hand in `bootstrap/app.go`, like every Arandu
module. `Authorize` is required: a receiver anybody could publish to is not a
configuration this package offers.

```go
import rtmp "github.com/hyz-is/arandu-rtmp"

receiver, err := rtmp.New(rtmp.Config{
	Addr: ":1935",
	Authorize: func(ctx context.Context, key string) bool {
		return cameras.HasIngestKey(key) // the application's own lookup
	},
})
if err != nil {
	return App{}, err
}
app.Register(receiver)
```

The framework starts it with the server. When the set of keys changes (a camera
removed, a key replaced), call `receiver.Revalidate(ctx)`: a revoked key is
dropped at once instead of when its camera next reconnects.

## What it reports

`receiver.Streams()` returns, per stream being published: the key, the
publisher's address, the codec, width and height read from the parameter sets,
the frames per second over the last five seconds, when the last frame arrived,
the bytes received and how many readers it has.

## Who may read

Only loopback, by default. `Config.Players` replaces that rule with a function
of the reader's address.

## Measured

On an Apple laptop, ten publishers of 1920x1080 H.264 at 10 fps (2.5 Mbit/s
each), each read back by one local ffmpeg: every stream held 10.0 fps, the
receiver used 0.6 to 2.4 % of one core and about 34 MB of memory.
