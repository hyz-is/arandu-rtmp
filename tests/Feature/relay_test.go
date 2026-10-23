package feature_test

import (
	"context"
	"net"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bluenviron/gortmplib"
	"github.com/bluenviron/gortmplib/pkg/codecs"

	rtmp "github.com/hyz-is/arandu-rtmp"
)

// A 1920x1080 baseline SPS and its PPS, as a camera would send them.
var h264Codec = &codecs.H264{
	SPS: []byte{
		0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02,
		0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04,
		0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9, 0x20,
	},
	PPS: []byte{0x08, 0x06, 0x07, 0x08},
}

var (
	keyFrame   = [][]byte{{0x65, 0x88, 0x84, 0x00}} // IDR slice
	deltaFrame = [][]byte{{0x41, 0x9a, 0x21, 0x00}} // non-IDR slice
)

// keys is an Authorizer whose allowed keys a test changes.
type keys struct {
	mu  sync.Mutex
	set map[string]bool
}

func (k *keys) allow(ctx context.Context, key string) bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.set[key]
}

func (k *keys) revoke(key string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.set, key)
}

func receiver(t *testing.T, cfg rtmp.Config) (*rtmp.Module, string) {
	t.Helper()
	m, err := rtmp.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = m.Serve(ctx, ln) }()
	t.Cleanup(func() {
		cancel()
		_ = m.Close(context.Background())
	})
	return m, ln.Addr().String()
}

type camera struct {
	c     *gortmplib.Client
	w     *gortmplib.Writer
	track *gortmplib.Track
	pts   time.Duration
}

// publish connects a camera to key; the error is the receiver's answer.
func publish(addr, key string) (*camera, error) {
	u, _ := url.Parse("rtmp://" + addr + "/live/" + key)
	c := &gortmplib.Client{URL: u, Publish: true}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Initialize(ctx); err != nil {
		return nil, err
	}
	track := &gortmplib.Track{Codec: h264Codec}
	w := &gortmplib.Writer{Conn: c, Tracks: []*gortmplib.Track{track}}
	if err := w.Initialize(); err != nil {
		c.Close()
		return nil, err
	}
	return &camera{c: c, w: w, track: track}, nil
}

// send writes one frame, 40 ms of stream time after the last, as a camera at
// 25 fps does.
func (cam *camera) send(au [][]byte) error {
	cam.pts += 40 * time.Millisecond
	return cam.w.WriteH264(cam.track, cam.pts, cam.pts, au)
}

// warm sends the first 2.4 s of stream: the receiver reads 2 s of a new
// publisher before it knows its tracks, as it would a real camera's.
func (cam *camera) warm(t *testing.T) {
	t.Helper()
	for i := range 60 {
		au := deltaFrame
		if i == 0 {
			au = keyFrame
		}
		if err := cam.send(au); err != nil {
			t.Fatal(err)
		}
	}
}

// play reads key back and counts what arrives; first records whether the first
// frame the reader saw was a key frame.
type player struct {
	frames atomic.Int64
	first  atomic.Int32 // 0 none yet, 1 key frame, 2 delta frame
}

func play(addr, key string) (*player, func(), error) {
	u, _ := url.Parse("rtmp://" + addr + "/live/" + key)
	c := &gortmplib.Client{URL: u}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Initialize(ctx); err != nil {
		return nil, nil, err
	}
	r := &gortmplib.Reader{Conn: c}
	if err := r.Initialize(); err != nil {
		c.Close()
		return nil, nil, err
	}
	p := &player{}
	for _, track := range r.Tracks() {
		if _, ok := track.Codec.(*codecs.H264); ok {
			r.OnDataH264(track, func(_, _ time.Duration, au [][]byte) {
				if p.first.Load() == 0 {
					if au[len(au)-1][0]&0x1f == 5 {
						p.first.Store(1)
					} else {
						p.first.Store(2)
					}
				}
				p.frames.Add(1)
			})
		}
	}
	go func() {
		for r.Read() == nil {
		}
	}()
	return p, c.Close, nil
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestAKeyTheInstallerDoesNotKnowCannotPublish(t *testing.T) {
	k := &keys{set: map[string]bool{"camera-key": true}}
	m, addr := receiver(t, rtmp.Config{Authorize: k.allow})
	if _, err := publish(addr, "guessed-key"); err == nil {
		t.Fatal("an unknown key published")
	}
	if len(m.Streams()) != 0 {
		t.Fatalf("a refused publisher left a stream: %+v", m.Streams())
	}
}

func TestAPublishedStreamReachesALocalReaderFromAKeyFrame(t *testing.T) {
	k := &keys{set: map[string]bool{"camera-key": true}}
	m, addr := receiver(t, rtmp.Config{Authorize: k.allow})
	cam, err := publish(addr, "camera-key")
	if err != nil {
		t.Fatalf("an allowed key is refused: %v", err)
	}
	defer cam.c.Close()
	cam.warm(t)
	eventually(t, "the stream", func() bool { return len(m.Streams()) == 1 })

	// The reader joins while the camera keeps sending, as the engine does.
	type joined struct {
		p     *player
		close func()
		err   error
	}
	got := make(chan joined, 1)
	go func() {
		p, closeReader, err := play(addr, "camera-key")
		got <- joined{p, closeReader, err}
	}()
	eventually(t, "the reader", func() bool { return m.Streams()[0].Readers == 1 })
	if err := cam.send(deltaFrame); err != nil {
		t.Fatal(err)
	}
	cam.warm(t)
	j := <-got
	if j.err != nil {
		t.Fatalf("a loopback reader is refused: %v", j.err)
	}
	defer j.close()
	cam.warm(t)
	// The client's reader spends the first frames finding the tracks, so the
	// key-frame start is proven inside the package (relay_internal_test.go);
	// here the video has to arrive.
	eventually(t, "frames at the reader", func() bool { return j.p.frames.Load() > 0 })

	s := m.Streams()[0]
	if s.Key != "camera-key" || s.Codec != "H264" || s.Width != 1920 || s.Height != 1080 || s.FPS <= 0 || s.LastFrame.IsZero() {
		t.Fatalf("stats: %+v", s)
	}
}

func TestAReaderTheFilterRefusesGetsNothing(t *testing.T) {
	k := &keys{set: map[string]bool{"camera-key": true}}
	_, addr := receiver(t, rtmp.Config{Authorize: k.allow, Players: func(net.Addr) bool { return false }})
	cam, err := publish(addr, "camera-key")
	if err != nil {
		t.Fatal(err)
	}
	defer cam.c.Close()
	cam.warm(t)
	if _, _, err := play(addr, "camera-key"); err == nil {
		t.Fatal("a reader the filter refuses read the stream")
	}
}

func TestARevokedKeyStopsAtOnce(t *testing.T) {
	k := &keys{set: map[string]bool{"camera-key": true}}
	m, addr := receiver(t, rtmp.Config{Authorize: k.allow})
	cam, err := publish(addr, "camera-key")
	if err != nil {
		t.Fatal(err)
	}
	defer cam.c.Close()
	cam.warm(t)
	eventually(t, "the stream", func() bool { return len(m.Streams()) == 1 })
	k.revoke("camera-key")
	if dropped := m.Revalidate(context.Background()); dropped != 1 {
		t.Fatalf("Revalidate dropped %d streams, want 1", dropped)
	}
	eventually(t, "the stream to go", func() bool { return len(m.Streams()) == 0 })
}

func TestANewerPublisherOfTheSameKeyReplacesTheOlder(t *testing.T) {
	k := &keys{set: map[string]bool{"camera-key": true}}
	m, addr := receiver(t, rtmp.Config{Authorize: k.allow})
	old, err := publish(addr, "camera-key")
	if err != nil {
		t.Fatal(err)
	}
	defer old.c.Close()
	old.warm(t)
	eventually(t, "the first stream", func() bool { return len(m.Streams()) == 1 })
	before := m.Streams()[0].Remote

	// A camera that rebooted connects again before its old connection timed out.
	again, err := publish(addr, "camera-key")
	if err != nil {
		t.Fatalf("the reconnecting camera is refused: %v", err)
	}
	defer again.c.Close()
	again.warm(t)
	eventually(t, "the newer publisher", func() bool {
		s := m.Streams()
		return len(s) == 1 && s[0].Remote != before
	})
}

func TestNewRefusesAServerAnybodyCouldPublishTo(t *testing.T) {
	if _, err := rtmp.New(rtmp.Config{}); err == nil {
		t.Fatal("a receiver without an Authorizer was built")
	}
}
