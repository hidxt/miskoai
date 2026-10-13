package library

import (
	"context"
	"errors"
	"github.com/hidxt/miskoai/internal/attachment"
	"io"
	"testing"
	"time"
)

func retain(t *testing.T, f fixture) {
	t.Helper()
	if e := f.m.WithMaintenance(context.Background(), func(ctx context.Context, v *Maintenance) error {
		rows, e := f.s.LibrarySnapshot(ctx)
		if e != nil {
			return e
		}
		return v.RetainCurrent(rows)
	}); e != nil {
		t.Fatal(e)
	}
}
func TestRetainedMaintenanceLifetime(t *testing.T) {
	f := setup(t)
	entered := make(chan struct{})
	unblock := make(chan struct{})
	returned := make(chan error, 1)
	closed := make(chan error, 1)
	// Register unblock and join before any held assertion.
	defer func() { close(unblock); <-returned; <-closed }()
	go func() {
		returned <- f.m.WithMaintenance(context.Background(), func(ctx context.Context, v *Maintenance) error {
			close(entered)
			<-ctx.Done()
			<-unblock
			return ctx.Err()
		})
	}()
	select {
	case <-entered:
	case e := <-returned:
		returned <- e
		go func() { closed <- f.m.Close() }()
		t.Fatal("callback did not enter", e)
	case <-time.After(time.Second):
		go func() { closed <- f.m.Close() }()
		t.Fatal("entry timeout")
	}
	go func() { closed <- f.m.Close() }()
	select {
	case e := <-closed:
		closed <- e
		t.Fatal("Close returned while callback held")
	case <-time.After(30 * time.Millisecond):
	}
}
func TestRetainedMaintenanceView(t *testing.T) {
	f := setup(t)
	var held *Maintenance
	e := f.m.WithMaintenance(context.Background(), func(ctx context.Context, v *Maintenance) error {
		held = v
		copy := *v
		if !errors.Is(copy.RetainCurrent(nil), ErrInvalid) {
			t.Error("copy accepted")
		}
		return v.RetainCurrent(nil)
	})
	if e != nil {
		t.Fatal(e)
	}
	if !errors.Is(held.RetainCurrent(nil), ErrInvalid) {
		t.Fatal("expired accepted")
	}
	var nilView *Maintenance
	if !errors.Is(nilView.RetainCurrent(nil), ErrInvalid) {
		t.Fatal("nil accepted")
	}
}

type retainedHeldReader struct {
	io.Reader
	entered chan struct{}
	unblock chan struct{}
	once    bool
}

func (r *retainedHeldReader) Read(b []byte) (int, error) {
	if !r.once {
		r.once = true
		close(r.entered)
		<-r.unblock
	}
	return r.Reader.Read(b)
}

type retainedHeldProcessor struct {
	Processor
	entered chan struct{}
	unblock chan struct{}
}

func (p retainedHeldProcessor) WithLocalImage(ctx context.Context, r io.Reader, n int64, consume func(attachment.LocalImage, attachment.Artifact) error) error {
	return p.Processor.WithLocalImage(ctx, &retainedHeldReader{Reader: r, entered: p.entered, unblock: p.unblock}, n, consume)
}
func TestRetainedMaintenanceHeldMedia(t *testing.T) {
	f := setup(t)
	upload(t, f, "gif")
	entered := make(chan struct{})
	unblock := make(chan struct{})
	returned := make(chan error, 1)
	closed := make(chan error, 1)
	f.m.processor = retainedHeldProcessor{Processor: f.p, entered: entered, unblock: unblock}
	defer func() {
		close(unblock)
		if e := <-returned; !errors.Is(e, context.Canceled) {
			t.Error("cancel missing", e)
		}
		<-closed
	}()
	go func() {
		returned <- f.m.WithMaintenance(context.Background(), func(ctx context.Context, v *Maintenance) error {
			rows, e := f.s.LibrarySnapshot(ctx)
			if e != nil {
				return e
			}
			return v.RetainCurrent(rows)
		})
	}()
	select {
	case <-entered:
	case e := <-returned:
		returned <- e
		go func() { closed <- f.m.Close() }()
		t.Fatal("reader did not enter", e)
	case <-time.After(time.Second):
		go func() { closed <- f.m.Close() }()
		t.Fatal("entry timeout")
	}
	go func() { closed <- f.m.Close() }()
	select {
	case e := <-closed:
		closed <- e
		t.Fatal("Close escaped reader")
	case <-time.After(30 * time.Millisecond):
	}
}
