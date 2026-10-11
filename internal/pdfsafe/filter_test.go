package pdfsafe

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/ascii85"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

func writeZlib(w io.Writer, p []byte) { z := zlib.NewWriter(w); _, _ = z.Write(p); _ = z.Close() }
func streamFixture(p []byte, filter object, param object) (value, *budget) {
	b := newBudget(context.Background())
	r := &reader{end: int64(len(p)), b: b}
	r.f = &checkedReaderAt{bytes.NewReader(p), b, int64(len(p))}
	return value{r, objptr{1, 0}, stream{hdr: dict{"Length": int64(len(p)), "Filter": filter, "DecodeParms": param}, ptr: objptr{1, 0}}}, b
}
func TestPDFFilterAggregateAndClose(t *testing.T) {
	t.Run("flate_EOF_sticky", func(t *testing.T) {
		var z bytes.Buffer
		writeZlib(&z, []byte("hello"))
		v, b := streamFixture(z.Bytes(), name("FlateDecode"), nil)
		s := v.reader()
		t.Cleanup(func() { _ = s.Close() })
		p, e := io.ReadAll(s)
		if e != nil || string(p) != "hello" {
			t.Fatal("flate decode failed")
		}
		var probe [1]byte
		n, e := s.Read(probe[:])
		if n != 0 || e != io.EOF {
			t.Fatal("successful decoder EOF not preserved")
		}
		if len(b.streams) != 0 || len(s.closers) != 1 || !s.closed {
			t.Fatal("actual Flate closer unowned")
		}
	})
	t.Run("all_stage", func(t *testing.T) {
		rows := make([]byte, 6<<20)
		for i := 0; i < len(rows); i += 4096 {
			rows[i] = 2
		}
		var z bytes.Buffer
		writeZlib(&z, rows)
		v, b := streamFixture(z.Bytes(), name("FlateDecode"), dict{"Predictor": int64(12), "Columns": int64(4095)})
		s := v.reader()
		t.Cleanup(func() { _ = s.Close() })
		mustFault(t, errLimit, func() { _, _ = io.Copy(io.Discard, s) })
		if b.decoded > maxDecoded || !s.closed || len(b.streams) != 0 {
			t.Fatal("aggregate stage failure leaked stream")
		}
	})
	t.Run("exact_and_probe", func(t *testing.T) {
		for _, remaining := range []int64{5, 4} {
			v, b := streamFixture([]byte("hello"), nil, nil)
			b.decoded = maxDecoded - remaining
			s := v.reader()
			t.Cleanup(func() { _ = s.Close() })
			if remaining == 5 {
				p, e := io.ReadAll(s)
				if e != nil || string(p) != "hello" || b.decoded != maxDecoded {
					t.Fatal("exact decoded ceiling failed")
				}
			} else {
				mustFault(t, errLimit, func() { _, _ = io.ReadAll(s) })
			}
			if len(b.streams) != 0 {
				t.Fatal("probe leaked")
			}
		}
	})
	t.Run("filter_chain", func(t *testing.T) {
		var a, z bytes.Buffer
		writeZlib(&a, []byte("hello"))
		writeZlib(&z, a.Bytes())
		v, b := streamFixture(z.Bytes(), array{name("FlateDecode"), name("FlateDecode")}, nil)
		s := v.reader()
		t.Cleanup(func() { _ = s.Close() })
		p, e := io.ReadAll(s)
		if e != nil || string(p) != "hello" || len(s.closers) != 2 || len(b.streams) != 0 {
			t.Fatal("real multi-decoder chain failed")
		}
	})
	t.Run("construction_error", func(t *testing.T) {
		var z bytes.Buffer
		writeZlib(&z, []byte("not zlib"))
		v, b := streamFixture(z.Bytes(), array{name("FlateDecode"), name("FlateDecode")}, nil)
		mustFault(t, errMalformed, func() { v.reader() })
		if len(b.streams) != 0 {
			t.Fatal("failed second decoder leaked first")
		}
	})
	t.Run("filter_limit", func(t *testing.T) {
		v, b := streamFixture([]byte("hello"), array{name("a"), name("b"), name("c"), name("d"), name("e")}, nil)
		mustFault(t, errLimit, func() { v.reader() })
		if len(b.streams) != 0 {
			t.Fatal("filter chain ceiling leaked")
		}
	})
	t.Run("unsupported", func(t *testing.T) {
		v, b := streamFixture([]byte("hello"), name("LZWDecode"), nil)
		mustFault(t, errUnsupported, func() { v.reader() })
		if len(b.streams) != 0 {
			t.Fatal("unsupported filter leaked")
		}
	})
	t.Run("ASCII85", func(t *testing.T) {
		raw := []byte("hello")
		enc := make([]byte, ascii85.MaxEncodedLen(len(raw)))
		n := ascii85.Encode(enc, raw)
		enc = append(enc[:n], '~', '>')
		v, b := streamFixture(enc, name("ASCII85Decode"), nil)
		s := v.reader()
		t.Cleanup(func() { _ = s.Close() })
		p, e := io.ReadAll(s)
		if e != nil || string(p) != "hello" || len(b.streams) != 0 {
			t.Fatal("ASCII85 decode failed")
		}
	})
}

type readCounter struct {
	r io.Reader
	n int
}

func (r *readCounter) Read(p []byte) (int, error) { r.n++; return r.r.Read(p) }
func TestPDFFilterRetainBeforeDecoder(t *testing.T) {
	var z bytes.Buffer
	writeZlib(&z, []byte("hello"))
	b := newBudget(context.Background())
	b.retained = maxRetained
	c := &readCounter{r: bytes.NewReader(z.Bytes())}
	mustFault(t, errLimit, func() { applyFilter(c, "FlateDecode", value{}, &streamReader{b: b}) })
	if c.n != 0 {
		t.Fatal("decoder allocated/read before retained admission")
	}
}

type heldReaderAt struct {
	entered, release chan struct{}
	once             sync.Once
	data             []byte
}

func (r *heldReaderAt) ReadAt(p []byte, off int64) (int, error) {
	r.once.Do(func() { close(r.entered) })
	<-r.release
	return bytes.NewReader(r.data).ReadAt(p, off)
}
func TestPDFCanceledActualReadJoins(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	b := newBudget(ctx)
	h := &heldReaderAt{entered: make(chan struct{}), release: make(chan struct{}), data: []byte("hello")}
	r := &reader{b: b, end: 5}
	r.f = &checkedReaderAt{h, b, 5}
	v := value{r, objptr{1, 0}, stream{hdr: dict{"Length": int64(5)}}}
	s := v.reader()
	done := make(chan struct{})
	var result interface{}
	var release sync.Once
	t.Cleanup(func() { cancel(); release.Do(func() { close(h.release) }); <-done; _ = s.Close() })
	go func() { defer func() { result = recover(); close(done) }(); _, _ = io.ReadAll(s) }()
	select {
	case <-h.entered:
	case <-time.After(time.Second):
		t.Fatal("actual reader did not enter")
	}
	cancel()
	select {
	case <-done:
		t.Fatal("returned before actual read released")
	default:
	}
	release.Do(func() { close(h.release) })
	// A closed completion channel allows cleanup to join even after a failed assertion.
	select {
	case <-done:
		f, ok := result.(fault)
		if !ok || !errors.Is(f.err, context.Canceled) {
			t.Errorf("cancellation not propagated: %T", result)
		}
	case <-time.After(time.Second):
		t.Fatal("reader did not complete after release")
	}
	if !s.closed || len(b.streams) != 0 {
		t.Fatal("canceled actual chain leaked")
	}
}

type closeSpy struct {
	io.Closer
	calls int
}

func (s *closeSpy) Close() error { s.calls++; return s.Closer.Close() }
func TestPDFActualDecoderCloseAndCancel(t *testing.T) {
	for _, mode := range []string{"success", "cancel", "unsupported_next"} {
		t.Run(mode, func(t *testing.T) {
			var z bytes.Buffer
			writeZlib(&z, []byte("hello"))
			v, b := streamFixture(z.Bytes(), name("FlateDecode"), nil)
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			b.ctx = ctx
			s := v.reader()
			spy := &closeSpy{Closer: s.closers[0]}
			s.closers[0] = spy
			t.Cleanup(func() { _ = s.Close() })
			switch mode {
			case "success":
				if _, e := io.Copy(io.Discard, s); e != nil {
					t.Fatal(e)
				}
			case "cancel":
				cancel()
				mustFault(t, context.Canceled, func() { s.Read(make([]byte, 1)) })
			case "unsupported_next":
				mustFault(t, errUnsupported, func() {
					func() {
						defer func() {
							if p := recover(); p != nil {
								_ = s.Close()
								panic(p)
							}
						}()
						applyFilter(s.r, "LZWDecode", value{}, s)
					}()
				})
			}
			if spy.calls != 1 || !s.closed || len(b.streams) != 0 {
				t.Fatal("actual zlib Close was not called exactly once")
			}
		})
	}
}
func TestPDFActiveStreamsAndEOFAttempts(t *testing.T) {
	v, b := streamFixture(nil, nil, nil)
	t.Cleanup(func() { _ = b.closeStreams() })
	for i := 0; i < maxStreams; i++ {
		v.reader()
	}
	mustFault(t, errLimit, func() { v.reader() })
	if len(b.streams) != maxStreams {
		t.Fatal("active stream cap moved")
	}
	if e := b.closeStreams(); e != nil || len(b.streams) != 0 {
		t.Fatal("stream release failed")
	}
	s := v.reader()
	var p [1]byte
	n, e := s.Read(p[:])
	if n != 0 || e != io.EOF {
		t.Fatal("empty raw stream failed")
	}
	before := b.work
	s.Read(p[:])
	if b.work <= before {
		t.Fatal("closed EOF attempts not metered")
	}
}
