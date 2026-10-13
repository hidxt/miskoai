package librarybundle

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func smallHeader() Header { return Header{File{2, digest([]byte("db"))}, []Asset{}} }
func TestLifecycleReleaseJoins(t *testing.T) {
	started := make(chan struct{})
	finish := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- Encode(context.Background(), io.Discard, smallHeader(), func(Record) (io.Reader, func() error, error) {
			return strings.NewReader("db"), func() error { close(started); <-finish; return nil }, nil
		})
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("release not reached")
	}
	select {
	case err := <-done:
		t.Fatalf("returned before release: %v", err)
	default:
	}
	close(finish)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("not joined")
	}
}
func TestLifecycleOpenErrorRelease(t *testing.T) {
	released := 0
	err := Encode(context.Background(), io.Discard, smallHeader(), func(Record) (io.Reader, func() error, error) {
		return nil, func() error { released++; return errors.New("release CANARY") }, errors.New("open CANARY")
	})
	if released != 1 || !errors.Is(err, ErrIO) || !errors.Is(err, ErrCleanup) || strings.Contains(err.Error(), "CANARY") {
		t.Fatalf("released=%d err=%v", released, err)
	}
}
func TestLifecycleCancellationCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	released := 0
	err := Encode(ctx, io.Discard, smallHeader(), func(Record) (io.Reader, func() error, error) {
		return &cancelReader{cancel: cancel}, func() error { released++; return errors.New("CANARY") }, nil
	})
	if released != 1 || !errors.Is(err, context.Canceled) || !errors.Is(err, ErrCleanup) || strings.Contains(err.Error(), "CANARY") {
		t.Fatalf("released=%d err=%v", released, err)
	}
}

type cancelReader struct{ cancel context.CancelFunc }

func (r *cancelReader) Read(p []byte) (int, error) { r.cancel(); p[0] = 'd'; return 1, nil }
func TestLifecycleBorrowedAndNil(t *testing.T) {
	in := &borrowedReader{Reader: bytes.NewReader(envelope(rawHeader([]byte("db"), nil), []byte("db")))}
	_, err := Decode(context.Background(), in, func(_ Record, r io.Reader) error { _, e := io.Copy(io.Discard, r); return e })
	if err != nil || in.closed {
		t.Fatalf("borrowed input err=%v closed=%v", err, in.closed)
	}
	out := &borrowedWriter{}
	if err := Encode(context.Background(), out, smallHeader(), func(Record) (io.Reader, func() error, error) {
		return strings.NewReader("db"), func() error { return nil }, nil
	}); err != nil || out.closed {
		t.Fatalf("borrowed output err=%v", err)
	}
	if err := Encode(context.Background(), io.Discard, smallHeader(), nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("nil open: %v", err)
	}
	if _, err := Decode(context.Background(), bytes.NewReader(nil), nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("nil consume: %v", err)
	}
}

type borrowedReader struct {
	*bytes.Reader
	closed bool
}

func (r *borrowedReader) Close() error { r.closed = true; return nil }

type borrowedWriter struct {
	bytes.Buffer
	closed bool
}

func (w *borrowedWriter) Close() error { w.closed = true; return nil }

func TestLifecycleMissingSourceDependencies(t *testing.T) {
	for _, kind := range []string{"nil_reader", "nil_release", "both_nil"} {
		t.Run(kind, func(t *testing.T) {
			released := 0
			err := Encode(context.Background(), io.Discard, smallHeader(), func(Record) (io.Reader, func() error, error) {
				var r io.Reader = strings.NewReader("db")
				var release func() error = func() error { released++; return nil }
				if kind != "nil_release" {
					r = nil
				}
				if kind != "nil_reader" {
					release = nil
				}
				return r, release, nil
			})
			want := 0
			if kind == "nil_reader" {
				want = 1
			}
			if !errors.Is(err, ErrInvalid) || released != want {
				t.Fatalf("released=%d err=%v", released, err)
			}
		})
	}
	if _, e := Decode(nil, bytes.NewReader(nil), func(Record, io.Reader) error { return nil }); !errors.Is(e, ErrInvalid) {
		t.Fatalf("nil context: %v", e)
	}
	if e := Encode(context.Background(), nil, smallHeader(), func(Record) (io.Reader, func() error, error) { t.Fatal("opened"); return nil, nil, nil }); !errors.Is(e, ErrInvalid) {
		t.Fatalf("nil output: %v", e)
	}
	if _, e := Decode(context.Background(), nil, func(Record, io.Reader) error { t.Fatal("consumed"); return nil }); !errors.Is(e, ErrInvalid) {
		t.Fatalf("nil input: %v", e)
	}
}
func TestLifecycleCancellationStopsReads(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r := &countCancelReader{cancel: cancel}
	released := 0
	e := Encode(ctx, io.Discard, smallHeader(), func(Record) (io.Reader, func() error, error) { return r, func() error { released++; return nil }, nil })
	if !errors.Is(e, context.Canceled) || r.reads != 1 || released != 1 {
		t.Fatalf("reads=%d release=%d err=%v", r.reads, released, e)
	}
	ctx2, cancel2 := context.WithCancel(context.Background())
	prefix := envelope(rawHeader([]byte("db"), nil), nil)
	r2 := &countCancelReader{cancel: cancel2}
	calls := 0
	got, e := Decode(ctx2, io.MultiReader(bytes.NewReader(prefix), r2), func(_ Record, r io.Reader) error { calls++; _, e := io.Copy(io.Discard, r); return e })
	if !errors.Is(e, context.Canceled) || r2.reads != 1 || calls != 1 || got.Database.Bytes != 0 {
		t.Fatalf("reads=%d calls=%d err=%v", r2.reads, calls, e)
	}
}

type countCancelReader struct {
	cancel context.CancelFunc
	reads  int
}

func (r *countCancelReader) Read(p []byte) (int, error) {
	r.reads++
	r.cancel()
	p[0] = 'd'
	return 1, nil
}
func TestLifecycleCleanupStopsNextRecord(t *testing.T) {
	h := smallHeader()
	h.Assets = []Asset{{strings.Repeat("a", 64), 1, digest([]byte("a"))}}
	calls := 0
	release := 0
	err := Encode(context.Background(), io.Discard, h, func(Record) (io.Reader, func() error, error) {
		calls++
		return strings.NewReader("db"), func() error { release++; return errors.New("CANARY") }, nil
	})
	if calls != 1 || release != 1 || !errors.Is(err, ErrCleanup) || strings.Contains(err.Error(), "CANARY") {
		t.Fatalf("calls=%d release=%d err=%v", calls, release, err)
	}
}
func TestLifecycleAlreadyCanceledAndDeadline(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		want := error(context.Canceled)
		if deadline {
			cancel()
			ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			want = context.DeadlineExceeded
		} else {
			cancel()
		}
		defer cancel()
		calls := 0
		err := Encode(ctx, io.Discard, smallHeader(), func(Record) (io.Reader, func() error, error) { calls++; return nil, nil, nil })
		if !errors.Is(err, want) || calls != 0 {
			t.Fatalf("calls=%d err=%v", calls, err)
		}
		_, err = Decode(ctx, bytes.NewReader(nil), func(Record, io.Reader) error { calls++; return nil })
		if !errors.Is(err, want) || calls != 0 {
			t.Fatalf("calls=%d err=%v", calls, err)
		}
	}
}
func TestLifecycleCancellationDuringRelease(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	err := Encode(ctx, io.Discard, smallHeader(), func(Record) (io.Reader, func() error, error) {
		return strings.NewReader("db"), func() error { cancel(); return errors.New("CANARY") }, nil
	})
	if !errors.Is(err, context.Canceled) || !errors.Is(err, ErrCleanup) || strings.Contains(err.Error(), "CANARY") {
		t.Fatal(err)
	}
}

func TestLifecycleReleaseFailureAlwaysCleanup(t *testing.T) {
	for _, releaseErr := range []error{ErrInvalid, ErrIO, context.Canceled, context.DeadlineExceeded} {
		err := Encode(context.Background(), io.Discard, smallHeader(), func(Record) (io.Reader, func() error, error) {
			return strings.NewReader("db"), func() error { return releaseErr }, nil
		})
		if !errors.Is(err, ErrCleanup) || !errors.Is(err, releaseErr) {
			t.Fatalf("release=%v err=%v", releaseErr, err)
		}
	}
}
func TestLifecycleReadCancellationKeepsIOClass(t *testing.T) {
	for _, where := range []string{"prefix", "payload", "probe"} {
		t.Run(where, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			tail := &cancelErrorReader{cancel: cancel}
			var input io.Reader = tail
			if where == "payload" {
				input = io.MultiReader(bytes.NewReader(envelope(rawHeader([]byte("db"), nil), nil)), tail)
			} else if where == "probe" {
				input = io.MultiReader(bytes.NewReader(envelope(rawHeader([]byte("db"), nil), []byte("db"))), tail)
			}
			_, err := Decode(ctx, input, func(_ Record, r io.Reader) error { _, e := io.Copy(io.Discard, r); return e })
			if !errors.Is(err, context.Canceled) || !errors.Is(err, ErrIO) || strings.Contains(err.Error(), "CANARY") {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

type cancelErrorReader struct{ cancel context.CancelFunc }

func (r *cancelErrorReader) Read([]byte) (int, error) { r.cancel(); return 0, errors.New("CANARY") }
