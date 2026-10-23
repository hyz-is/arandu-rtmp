package rtmp

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"time"
)

// The defaults for the optional settings. They are constants rather than
// literals inside Config.withDefaults, so the value a reader finds here is the
// value the package uses.
const (
	// DefaultAddr is where the server listens when Config leaves Addr empty:
	// every interface, on the port RTMP clients assume.
	DefaultAddr = ":1935"
	// DefaultApp is the application name a publisher has to use, the "live"
	// in rtmp://host/live/<key>.
	DefaultApp = "live"
	// DefaultIdleTimeout is how long a publisher may send nothing before it is
	// dropped. A camera that loses its uplink usually leaves the TCP
	// connection open; without a deadline its stream would look alive forever.
	DefaultIdleTimeout = 10 * time.Second
	// DefaultHandshakeTimeout bounds the RTMP handshake and the connect/publish
	// commands, so a port scanner holds a goroutine for seconds, not forever.
	DefaultHandshakeTimeout = 10 * time.Second
)

// Authorizer decides whether key may publish. It is the installer's: the
// package knows nothing about cameras, tenants or where keys are stored. It
// is asked when a publisher connects and again by Revalidate.
type Authorizer func(ctx context.Context, key string) bool

// PlayerFilter decides whether a connection from addr may read a stream. Nil
// means only loopback: the stream is meant for the process beside the server
// (an engine that reads it back with ffmpeg), not for the internet.
type PlayerFilter func(addr net.Addr) bool

// Config is what the application passes when it wires this package.
//
// A typed struct rather than a map: a misspelled key in a map is a setting that
// silently keeps its default. Authorize is the only field without one, because
// a server that lets anybody publish is not a configuration this package
// offers.
type Config struct {
	// Addr is the TCP address to listen on. Empty means DefaultAddr.
	Addr string
	// App is the application name publishers use. Empty means DefaultApp.
	App string
	// Authorize decides which stream keys may publish. Required.
	Authorize Authorizer
	// Players decides who may read a stream back. Nil means loopback only.
	Players PlayerFilter
	// IdleTimeout and HandshakeTimeout bound a silent publisher and a slow
	// handshake. Zero means the defaults.
	IdleTimeout, HandshakeTimeout time.Duration
	// RevalidateEvery asks Authorize again about every stream being published
	// at this interval, so a revoked key stops even when nobody calls
	// Revalidate. Zero leaves it to the installer.
	RevalidateEvery time.Duration
	// Logger receives one line per publisher that connects, is refused or
	// leaves. Nil means slog.Default().
	Logger *slog.Logger
}

// ErrNoAuthorizer is a Config without Authorize.
var ErrNoAuthorizer = errors.New("rtmp: Config.Authorize is required")

func (c Config) withDefaults() (Config, error) {
	if c.Authorize == nil {
		return c, ErrNoAuthorizer
	}
	if c.Addr == "" {
		c.Addr = DefaultAddr
	}
	if c.App == "" {
		c.App = DefaultApp
	}
	if c.Players == nil {
		c.Players = Loopback
	}
	if c.IdleTimeout <= 0 {
		c.IdleTimeout = DefaultIdleTimeout
	}
	if c.HandshakeTimeout <= 0 {
		c.HandshakeTimeout = DefaultHandshakeTimeout
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	return c, nil
}

// Loopback is the default PlayerFilter: only connections from this host.
func Loopback(addr net.Addr) bool {
	tcp, ok := addr.(*net.TCPAddr)
	return ok && tcp.IP.IsLoopback()
}
