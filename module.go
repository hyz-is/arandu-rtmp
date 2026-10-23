// Package rtmp receives RTMP streams that cameras, encoders and recorders push,
// and hands each one back to a reader on the same host.
//
// A camera behind carrier NAT or a 4G modem cannot be reached by an engine that
// pulls RTSP, but it can open a connection outwards: it publishes to
// rtmp://host/live/<key>, the package asks the installer whether that key may
// publish, and the engine beside the server plays rtmp://127.0.0.1/live/<key>
// back with the reader it already has. The key is the stream's name and its
// credential at once; the installer decides which keys exist.
//
// The package is a relay, not a media server: no recording, no transcoding, no
// players on the internet. Video is forwarded as it arrives, and a reader that
// joins late starts at the next key frame, so the first picture it decodes is a
// whole one.
package rtmp

import (
	"context"
	"errors"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/arandu-io/framework/http"
)

// Module is the RTMP receiver. The application builds it with New, registers
// it like any module, and the framework starts it with the server.
type Module struct {
	cfg Config

	mu      sync.Mutex
	streams map[string]*stream
	ln      net.Listener
	conns   sync.WaitGroup
}

// New validates cfg and returns the receiver. It listens only when started.
func New(cfg Config) (*Module, error) {
	cfg, err := cfg.withDefaults()
	if err != nil {
		return nil, err
	}
	return &Module{cfg: cfg, streams: map[string]*stream{}}, nil
}

// Name identifies the module.
func (m *Module) Name() string { return "rtmp" }

// Routes registers nothing: the receiver speaks RTMP on its own port, and its
// state is read through Streams by whoever renders it.
func (m *Module) Routes(*http.Router) {}

// Start listens on Config.Addr and serves until ctx ends. A port that cannot be
// bound is an error here, at boot, rather than a receiver that silently is not
// there.
func (m *Module) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", m.cfg.Addr)
	if err != nil {
		return err
	}
	go func() { _ = m.Serve(ctx, ln) }()
	if every := m.cfg.RevalidateEvery; every > 0 {
		go func() {
			tick := time.NewTicker(every)
			defer tick.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-tick.C:
					m.Revalidate(ctx)
				}
			}
		}()
	}
	return nil
}

// Serve accepts connections on ln until ctx ends or ln is closed. Start calls
// it; a test calls it with a listener of its own.
func (m *Module) Serve(ctx context.Context, ln net.Listener) error {
	m.mu.Lock()
	m.ln = ln
	m.mu.Unlock()
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) || ctx.Err() != nil {
				return nil
			}
			continue
		}
		m.conns.Add(1)
		go func() {
			defer m.conns.Done()
			m.handle(ctx, conn)
		}()
	}
}

// Addr is the address the receiver listens on, once it does.
func (m *Module) Addr() net.Addr {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ln == nil {
		return nil
	}
	return m.ln.Addr()
}

// Close stops listening and drops every publisher and reader.
func (m *Module) Close(context.Context) error {
	m.mu.Lock()
	ln := m.ln
	streams := make([]*stream, 0, len(m.streams))
	for _, s := range m.streams {
		streams = append(streams, s)
	}
	m.mu.Unlock()
	if ln != nil {
		_ = ln.Close()
	}
	for _, s := range streams {
		s.stop()
	}
	m.conns.Wait()
	return nil
}

// Streams reports every stream being published, ordered by key.
func (m *Module) Streams() []Stats {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Stats, 0, len(m.streams))
	for _, s := range m.streams {
		out = append(out, s.stats())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// Revalidate asks Authorize again about every stream being published and drops
// the ones it no longer allows. The installer calls it when the set of keys
// changes -- a camera removed, a key replaced -- so a revoked key stops at once
// instead of when its camera next reconnects. It returns how many it dropped.
func (m *Module) Revalidate(ctx context.Context) int {
	m.mu.Lock()
	streams := make([]*stream, 0, len(m.streams))
	for _, s := range m.streams {
		streams = append(streams, s)
	}
	m.mu.Unlock()
	dropped := 0
	for _, s := range streams {
		if !m.cfg.Authorize(ctx, s.key) {
			s.stop()
			dropped++
		}
	}
	return dropped
}
