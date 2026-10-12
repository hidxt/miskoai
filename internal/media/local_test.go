package media

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
)

type localReaderFunc func([]byte) (int, error)

func (f localReaderFunc) Read(p []byte) (int, error) { return f(p) }

func TestStageLocalExactCompletedPrivate(t *testing.T) {
	for _, size := range []int{1, 4097, 4 << 20} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			dir := mediaDir(t)
			raw := bytes.Repeat([]byte{42}, size)
			reads := 0
			r := bytes.NewReader(raw)
			a, e := StageLocal(context.Background(), dir, localReaderFunc(func(p []byte) (int, error) {
				reads++
				if len(p) > 4096 {
					t.Fatal("unbounded scratch")
				}
				return r.Read(p)
			}), int64(size))
			if e != nil || a == nil {
				t.Fatalf("completed artifact: %v", e)
			}
			defer a.Close()
			if a.Size() != int64(size) {
				t.Fatal("size")
			}
			got := make([]byte, size)
			if n, e := a.ReadAt(got, 0); n != size || e != nil || !bytes.Equal(got, raw) {
				t.Fatalf("bytes %d %v", n, e)
			}
			if reads < 2 {
				t.Fatal("missing EOF probe")
			}
			if e = a.Close(); e != nil {
				t.Fatal(e)
			}
			files, e := os.ReadDir(dir)
			if e != nil || len(files) != 0 {
				t.Fatalf("cleanup %v %d", e, len(files))
			}
		})
	}
}

func TestStageLocalPartialAndTerminalEOF(t *testing.T) {
	dir := mediaDir(t)
	raw := []byte("partial reader bytes")
	offset := 0
	a, e := StageLocal(context.Background(), dir, localReaderFunc(func(p []byte) (int, error) {
		if offset == len(raw) {
			return 0, io.EOF
		}
		p[0] = raw[offset]
		offset++
		if offset == len(raw) {
			return 1, io.EOF
		}
		return 1, nil
	}), int64(len(raw)))
	if e != nil || a == nil {
		t.Fatalf("partial EOF %v", e)
	}
	defer a.Close()
	got := make([]byte, len(raw))
	if n, e := a.ReadAt(got, 0); n != len(got) || e != nil || !bytes.Equal(got, raw) {
		t.Fatal("partial bytes")
	}
}

func TestStageLocalHardlinkRefusal(t *testing.T) {
	dir := mediaDir(t)
	var linked string
	a, e := stageLocal(context.Background(), dir, bytes.NewReader([]byte("owned")), 5, spoolOps{beforePublish: func(p string) {
		linked = p + ".hardlink"
		if e := os.Link(p, linked); e != nil {
			t.Fatal(e)
		}
	}})
	if a != nil {
		defer a.Close()
	}
	if !errors.Is(e, ErrPrivate) {
		t.Fatalf("hardlink admitted %v", e)
	}
	got, e := os.ReadFile(linked)
	if e != nil || string(got) != "owned" {
		t.Fatalf("retained link changed %v", e)
	}
}

func TestStageLocalFailedCleanupTransfersOwnership(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dir := mediaDir(t)
	var path string
	a, e := stageLocal(ctx, dir, localReaderFunc(func([]byte) (int, error) { cancel(); return 0, io.EOF }), 1, spoolOps{beforeCleanup: func(p string) { path = p; replaceSpool(t, p) }})
	if !errors.Is(e, context.Canceled) || !errors.Is(e, ErrPrivate) {
		t.Fatalf("cleanup classes %v", e)
	}
	if a == nil {
		t.Fatal("failed cleanup ownership must transfer even alongside error")
	}
	if !errors.Is(a.Close(), ErrPrivate) {
		t.Fatal("close lost cleanup refusal")
	}
	if _, e := a.ReadAt(make([]byte, 1), 0); !errors.Is(e, ErrIO) {
		t.Fatal("failed stage exposed bytes")
	}
	replacementRemains(t, path)
}

func TestStageLocalStreamRefusal(t *testing.T) {
	for _, name := range []string{"truncated", "trailing", "reader-error", "zero-progress", "invalid-count", "terminal-error", "probe-error", "canceled-read", "canceled-probe"} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			dir := mediaDir(t)
			want := ErrInvalid
			input := io.Reader(bytes.NewReader([]byte("abc")))
			size := int64(4)
			calls := 0
			switch name {
			case "trailing":
				size = 2
			case "reader-error":
				want = ErrIO
				input = localReaderFunc(func([]byte) (int, error) { return 0, errors.New("RAW-LOCAL-CANARY") })
			case "zero-progress":
				want = ErrIO
				input = localReaderFunc(func([]byte) (int, error) { return 0, nil })
			case "invalid-count":
				want = ErrIO
				input = localReaderFunc(func(p []byte) (int, error) { return len(p) + 1, nil })
			case "terminal-error":
				want = ErrIO
				size = 1
				input = localReaderFunc(func(p []byte) (int, error) { p[0] = 1; return 1, errors.New("RAW-LOCAL-CANARY") })
			case "probe-error":
				want = ErrIO
				size = 1
				input = localReaderFunc(func(p []byte) (int, error) {
					calls++
					if calls == 1 {
						p[0] = 1
						return 1, nil
					}
					return 0, errors.New("RAW-LOCAL-CANARY")
				})
			case "canceled-read", "canceled-probe":
				want = context.Canceled
				size = 1
				input = localReaderFunc(func(p []byte) (int, error) {
					calls++
					if name == "canceled-read" || calls > 1 {
						cancel()
						return 0, errors.New("RAW-LOCAL-CANARY")
					}
					p[0] = 1
					return 1, nil
				})
			}
			a, e := StageLocal(ctx, dir, input, size)
			if a != nil {
				defer a.Close()
			}
			if !errors.Is(e, want) || strings.Contains(e.Error(), "CANARY") {
				t.Fatalf("classification %v want %v", e, want)
			}
			files, e := os.ReadDir(dir)
			if e != nil || len(files) != 0 {
				t.Fatalf("partial cleanup %v %d", e, len(files))
			}
		})
	}
}

func TestStageLocalPreflightNoRead(t *testing.T) {
	for _, name := range []string{"nil-context", "nil-reader", "zero", "negative", "oversized", "canceled"} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			size := int64(1)
			input := io.Reader(localReaderFunc(func([]byte) (int, error) { t.Fatal("preflight read"); return 0, nil }))
			want := ErrInvalid
			switch name {
			case "nil-context":
				ctx = nil
			case "nil-reader":
				input = nil
			case "zero":
				size = 0
			case "negative":
				size = -1
			case "oversized":
				size = 4<<20 + 1
				want = ErrLimit
			case "canceled":
				c, cancel := context.WithCancel(ctx)
				cancel()
				ctx = c
				want = context.Canceled
			}
			dir := mediaDir(t)
			a, e := StageLocal(ctx, dir, input, size)
			if a != nil || !errors.Is(e, want) {
				t.Fatalf("preflight %v", e)
			}
			if _, e = os.Stat(dir); !os.IsNotExist(e) {
				t.Fatal("preflight created directory")
			}
		})
	}
}

func TestStageLocalOwnedFailureCleanup(t *testing.T) {
	for _, name := range []string{"short-write", "sync", "close", "replace-publish", "replace-cleanup"} {
		t.Run(name, func(t *testing.T) {
			dir := mediaDir(t)
			ops := spoolOps{}
			want := ErrIO
			var path string
			switch name {
			case "short-write":
				ops.write = func(*os.File, []byte) (int, error) { return 0, nil }
			case "sync":
				ops.sync = func(*os.File) error { return errors.New("RAW-LOCAL-CANARY") }
			case "close":
				ops.close = func(*os.File) error { return errors.New("RAW-LOCAL-CANARY") }
			case "replace-publish":
				want = ErrPrivate
				ops.beforePublish = func(p string) { path = p; replaceSpool(t, p) }
			case "replace-cleanup":
				want = ErrPrivate
				ops.write = func(*os.File, []byte) (int, error) { return 0, ErrIO }
				ops.beforeCleanup = func(p string) { path = p; replaceSpool(t, p) }
			}
			a, e := stageLocal(context.Background(), dir, bytes.NewReader([]byte("abc")), 3, ops)
			if a != nil {
				defer a.Close()
			}
			if !errors.Is(e, want) || strings.Contains(e.Error(), "CANARY") {
				t.Fatalf("owned error %v", e)
			}
			if path != "" {
				replacementRemains(t, path)
			} else {
				files, e := os.ReadDir(dir)
				if e != nil || len(files) != 0 {
					t.Fatalf("cleanup %v %d", e, len(files))
				}
			}
		})
	}
}
