package library

import (
	"bytes"
	"context"
	"errors"
	"github.com/hidxt/miskoai/internal/attachment"
	"github.com/hidxt/miskoai/internal/storage"
	"io"
	"os"
	"testing"
	"time"
)

func assertHeld(t *testing.T, ch <-chan error) {
	t.Helper()
	select {
	case e := <-ch:
		t.Fatal("lifetime returned early", e)
	case <-time.After(25 * time.Millisecond):
	}
}
func await(t *testing.T, ch <-chan error) error {
	t.Helper()
	select {
	case e := <-ch:
		return e
	case <-time.After(5 * time.Second):
		t.Fatal("operation did not join")
		return nil
	}
}
func TestLifecycleCloseJoinsReaderAndRejectsEntrants(t *testing.T) {
	f := setup(t)
	entered, allow := make(chan struct{}), make(chan struct{})
	r := readerFunc(func([]byte) (int, error) { close(entered); <-allow; return 0, io.EOF })
	op := make(chan error, 1)
	go func() {
		_, _, e := f.m.Upload(context.Background(), f.l, scope, r, 1, storage.LibraryMetadata{})
		op <- e
	}()
	<-entered
	second := make(chan error, 1)
	go func() { second <- f.m.RecordUse(context.Background(), f.l, scope, 1, string(make([]byte, 64)), 1) }() // invalid arguments do not enter admission
	if e := await(t, second); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
	closer := make(chan error, 1)
	go func() { closer <- f.m.Close() }()
	assertHeld(t, closer)
	closer2 := make(chan error, 1)
	go func() { closer2 <- f.m.Close() }()
	assertHeld(t, closer2)
	assertHeld(t, op)
	if e := f.m.WithAsset(context.Background(), f.l, scope, 1, false, consumeNothing); !errors.Is(e, ErrClosed) {
		t.Fatal("closed admission", e)
	}
	close(allow)
	if e := await(t, op); !errors.Is(e, context.Canceled) {
		t.Fatal("cancel reader", e)
	}
	if e := await(t, closer); e != nil {
		t.Fatal(e)
	}
	if e := await(t, closer2); e != nil {
		t.Fatal(e)
	}
	if e := f.m.Close(); e != nil {
		t.Fatal(e)
	}
	raw := rawImage(t, "png", 10)
	if e := f.p.WithLocalImage(context.Background(), bytes.NewReader(raw), int64(len(raw)), func(attachment.LocalImage, attachment.Artifact) error { return nil }); e != nil {
		t.Fatal("borrowed processor closed", e)
	}
	if _, e := f.s.LibrarySnapshot(context.Background()); e != nil {
		t.Fatal("borrowed store closed", e)
	}
}
func TestLifecycleConsumerAndSharedMediaAdmission(t *testing.T) {
	f := setup(t)
	a := upload(t, f, "png")
	entered, allow := make(chan struct{}), make(chan struct{})
	op := make(chan error, 1)
	go func() {
		op <- f.m.WithAsset(context.Background(), f.l, scope, a.ID, false, func(storage.LibraryAsset, attachment.LocalImage, attachment.Artifact) error {
			close(entered)
			<-allow
			return nil
		})
	}()
	<-entered
	second := make(chan error, 1)
	go func() { second <- f.m.Update(context.Background(), f.l, scope, a.ID, storage.LibraryMetadata{}) }()
	assertHeld(t, second)
	shared := make(chan error, 1)
	raw := rawImage(t, "jpeg", 10)
	go func() {
		shared <- f.p.WithLocalImage(context.Background(), bytes.NewReader(raw), int64(len(raw)), func(attachment.LocalImage, attachment.Artifact) error { return nil })
	}()
	assertHeld(t, shared)
	closer := make(chan error, 1)
	go func() { closer <- f.m.Close() }()
	assertHeld(t, closer)
	if e := await(t, second); !errors.Is(e, ErrClosed) {
		t.Fatal("waiting entrant", e)
	}
	close(allow)
	if e := await(t, op); !errors.Is(e, context.Canceled) {
		t.Fatal("consumer cancellation", e)
	}
	if e := await(t, shared); e != nil {
		t.Fatal("shared admission", e)
	}
	if e := await(t, closer); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(f.m.path(a)); e != nil {
		t.Fatal("persistent artifact removed", e)
	}
}
func TestLifecycleJoinsProcessorCleanupReturn(t *testing.T) {
	f := setup(t)
	a := upload(t, f, "png")
	cleanup, allow := make(chan struct{}), make(chan struct{})
	f.m.processor = processorFunc(func(c context.Context, r io.Reader, n int64, fn func(attachment.LocalImage, attachment.Artifact) error) error {
		e := f.p.WithLocalImage(c, r, n, fn)
		close(cleanup)
		<-allow
		return e
	})
	op := make(chan error, 1)
	go func() { op <- f.m.WithAsset(context.Background(), f.l, scope, a.ID, false, consumeNothing) }()
	<-cleanup
	closer := make(chan error, 1)
	go func() { closer <- f.m.Close() }()
	assertHeld(t, closer)
	assertHeld(t, op)
	close(allow)
	if e := await(t, op); !errors.Is(e, context.Canceled) {
		t.Fatal("cancel during cleanup", e)
	}
	if e := await(t, closer); e != nil {
		t.Fatal(e)
	}
}
