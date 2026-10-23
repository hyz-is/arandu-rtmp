package rtmp

import (
	"net"
	"testing"

	"github.com/bluenviron/gortmplib"
)

// TestAReaderStartsAtAKeyFrame: a reader that joins between key frames is sent
// nothing until the next one, so its decoder never starts on a partial picture.
func TestAReaderStartsAtAKeyFrame(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	s := &stream{done: make(chan struct{})}
	rd := &reader{conn: a, waiting: true}
	s.readers = []*reader{rd}

	var sent []bool
	forward := func(random bool) {
		s.frame(random, func(*gortmplib.Writer) error {
			sent = append(sent, random)
			return nil
		})
	}
	for _, random := range []bool{false, false, true, false, true, false} {
		forward(random)
	}
	want := []bool{true, false, true, false}
	if len(sent) != len(want) {
		t.Fatalf("forwarded %v, want %v", sent, want)
	}
	for i := range want {
		if sent[i] != want[i] {
			t.Fatalf("forwarded %v, want %v", sent, want)
		}
	}
	if rd.waiting {
		t.Fatal("the reader still waits after a key frame")
	}
}
