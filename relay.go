package rtmp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/gortmplib"
	"github.com/bluenviron/gortmplib/pkg/codecs"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
)

// fpsWindow is how far back the frame rate of a stream is measured.
const fpsWindow = 5 * time.Second

// Stats is what the receiver knows about one stream being published.
type Stats struct {
	// Key is the stream key the publisher used.
	Key string
	// Remote is the publisher's address.
	Remote string
	// Codec is the video codec: "H264" or "H265".
	Codec string
	// Width and Height come from the stream's parameter sets; zero when the
	// codec does not carry them in a form this package reads.
	Width, Height int
	// FPS is the video frames received per second over the last five seconds.
	FPS float64
	// Since is when the publisher connected; LastFrame when its last video
	// frame arrived.
	Since, LastFrame time.Time
	// BytesReceived counts what the publisher sent.
	BytesReceived uint64
	// Readers is how many connections are reading the stream back.
	Readers int
}

// stream is one key being published: its publisher, its video track and the
// readers it is forwarded to.
type stream struct {
	key       string
	publisher *gortmplib.ServerConn
	conn      net.Conn
	tracks    []*gortmplib.Track
	codec     string
	width     int
	height    int
	since     time.Time

	mu      sync.Mutex
	readers []*reader
	frames  []time.Time
	last    time.Time
	done    chan struct{}
	once    sync.Once
}

// reader is one connection reading a stream back. It waits for a key frame
// before it is sent anything, so its decoder never starts on a partial picture.
type reader struct {
	w       *gortmplib.Writer
	conn    net.Conn
	waiting bool
}

func (s *stream) stop() {
	s.once.Do(func() {
		close(s.done)
		_ = s.conn.Close()
	})
}

func (s *stream) stats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := time.Now().Add(-fpsWindow)
	n := 0
	for _, t := range s.frames {
		if t.After(cutoff) {
			n++
		}
	}
	return Stats{
		Key: s.key, Remote: s.conn.RemoteAddr().String(), Codec: s.codec, Width: s.width, Height: s.height,
		FPS: float64(n) / fpsWindow.Seconds(), Since: s.since, LastFrame: s.last,
		BytesReceived: s.publisher.BytesReceived(), Readers: len(s.readers),
	}
}

// frame records a video frame and forwards it to every reader that is past its
// first key frame. A reader whose write fails is dropped.
func (s *stream) frame(random bool, write func(*gortmplib.Writer) error) {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.last = now
	cutoff := now.Add(-fpsWindow)
	keep := s.frames[:0]
	for _, t := range s.frames {
		if t.After(cutoff) {
			keep = append(keep, t)
		}
	}
	s.frames = append(keep, now)
	alive := s.readers[:0]
	for _, r := range s.readers {
		if r.waiting && !random {
			alive = append(alive, r)
			continue
		}
		r.waiting = false
		if err := write(r.w); err != nil {
			_ = r.conn.Close()
			continue
		}
		alive = append(alive, r)
	}
	s.readers = alive
}

// handle runs one connection: the handshake, then publish or play.
func (m *Module) handle(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(m.cfg.HandshakeTimeout))
	sc := &gortmplib.ServerConn{RW: conn}
	if err := sc.Initialize(); err != nil {
		return
	}
	if err := sc.AcceptConn(); err != nil {
		return
	}
	key, err := m.streamKey(sc)
	if err != nil {
		_ = sc.RejectAction()
		m.cfg.Logger.Info("rtmp: refused", "remote", conn.RemoteAddr().String(), "reason", err.Error())
		return
	}
	if sc.Publish {
		m.publish(ctx, sc, conn, key)
		return
	}
	m.play(sc, conn, key)
}

// streamKey reads the key out of rtmp://host/<app>/<key>.
func (m *Module) streamKey(sc *gortmplib.ServerConn) (string, error) {
	if sc.URL == nil {
		return "", errors.New("no stream")
	}
	parts := strings.Split(strings.Trim(sc.URL.Path, "/"), "/")
	if len(parts) != 2 || parts[0] != m.cfg.App || parts[1] == "" {
		return "", fmt.Errorf("not %s/<key>", m.cfg.App)
	}
	return parts[1], nil
}

// publish authorizes the key and forwards the publisher's video until it
// stops, is replaced by a newer publisher of the same key, or is revoked.
func (m *Module) publish(ctx context.Context, sc *gortmplib.ServerConn, conn net.Conn, key string) {
	log := m.cfg.Logger.With("remote", conn.RemoteAddr().String(), "key", redact(key))
	if !m.cfg.Authorize(ctx, key) {
		_ = sc.RejectAction()
		log.Info("rtmp: publish refused")
		return
	}
	r := &gortmplib.Reader{Conn: sc}
	if err := r.Initialize(); err != nil {
		log.Info("rtmp: publish failed", "error", err.Error())
		return
	}
	s := &stream{key: key, publisher: sc, conn: conn, since: time.Now(), done: make(chan struct{})}
	for _, t := range r.Tracks() {
		switch c := t.Codec.(type) {
		case *codecs.H264:
			s.tracks, s.codec = []*gortmplib.Track{t}, "H264"
			var sps h264.SPS
			if sps.Unmarshal(c.SPS) == nil {
				s.width, s.height = sps.Width(), sps.Height()
			}
			r.OnDataH264(t, func(pts, dts time.Duration, au [][]byte) {
				s.frame(h264.IsRandomAccess(au), func(w *gortmplib.Writer) error { return w.WriteH264(t, pts, dts, au) })
			})
		case *codecs.H265:
			s.tracks, s.codec = []*gortmplib.Track{t}, "H265"
			var sps h265.SPS
			if sps.Unmarshal(c.SPS) == nil {
				s.width, s.height = sps.Width(), sps.Height()
			}
			r.OnDataH265(t, func(pts, dts time.Duration, au [][]byte) {
				s.frame(h265.IsRandomAccess(au), func(w *gortmplib.Writer) error { return w.WriteH265(t, pts, dts, au) })
			})
		}
		if s.codec != "" {
			break
		}
	}
	if s.codec == "" {
		log.Info("rtmp: publish refused", "reason", "no H264 or H265 video")
		return
	}

	m.mu.Lock()
	previous := m.streams[key]
	m.streams[key] = s
	m.mu.Unlock()
	if previous != nil {
		previous.stop()
	}
	log.Info("rtmp: publishing", "codec", s.codec, "width", s.width, "height", s.height)
	defer func() {
		m.mu.Lock()
		if m.streams[key] == s {
			delete(m.streams, key)
		}
		m.mu.Unlock()
		s.stop()
		s.mu.Lock()
		for _, rd := range s.readers {
			_ = rd.conn.Close()
		}
		s.mu.Unlock()
		log.Info("rtmp: publisher left")
	}()

	for {
		_ = conn.SetDeadline(time.Now().Add(m.cfg.IdleTimeout))
		if err := r.Read(); err != nil {
			return
		}
		select {
		case <-s.done:
			return
		default:
		}
	}
}

// play forwards a published stream to a reader the PlayerFilter allows.
func (m *Module) play(sc *gortmplib.ServerConn, conn net.Conn, key string) {
	if !m.cfg.Players(conn.RemoteAddr()) {
		_ = sc.RejectAction()
		return
	}
	m.mu.Lock()
	s := m.streams[key]
	m.mu.Unlock()
	if s == nil {
		_ = sc.RejectAction()
		return
	}
	w := &gortmplib.Writer{Conn: sc, Tracks: s.tracks}
	if err := w.Initialize(); err != nil {
		return
	}
	rd := &reader{w: w, conn: conn, waiting: true}
	s.mu.Lock()
	s.readers = append(s.readers, rd)
	s.mu.Unlock()
	_ = conn.SetDeadline(time.Time{})
	defer func() {
		s.mu.Lock()
		for i, x := range s.readers {
			if x == rd {
				s.readers = append(s.readers[:i], s.readers[i+1:]...)
				break
			}
		}
		s.mu.Unlock()
	}()
	select {
	case <-s.done:
	case <-waitErr(w):
	}
}

func waitErr(w *gortmplib.Writer) <-chan error {
	ch := make(chan error, 1)
	go func() { ch <- w.Wait() }()
	return ch
}

// redact keeps a key out of the logs: its first four characters are enough to
// tell two streams apart.
func redact(key string) string {
	if len(key) <= 4 {
		return "…"
	}
	return key[:4] + "…"
}
