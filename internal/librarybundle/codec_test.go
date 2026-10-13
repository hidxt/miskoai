package librarybundle

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestCodecRoundTrip(t *testing.T) {
	for _, assets := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty", true: "asset"}[assets], func(t *testing.T) {
			db := []byte("database synthetic")
			img := []byte("image synthetic")
			h := Header{Database: File{int64(len(db)), digest(db)}, Assets: []Asset{}}
			if assets {
				h.Assets = append(h.Assets, Asset{strings.Repeat("a", 64), int64(len(img)), digest(img)})
			}
			var out bytes.Buffer
			release := 0
			err := Encode(context.Background(), &out, h, func(r Record) (io.Reader, func() error, error) {
				p := db
				if r.Kind == Image {
					p = img
				}
				return &fragmentReader{bytes.NewReader(p)}, func() error { release++; return nil }, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			want := 1 + len(h.Assets)
			if release != want {
				t.Fatalf("release=%d", release)
			}
			calls := 0
			got, err := Decode(context.Background(), &fragmentReader{bytes.NewReader(out.Bytes())}, func(r Record, in io.Reader) error {
				p, err := io.ReadAll(in)
				expected := db
				if r.Kind == Image {
					expected = img
					if r.Key != h.Assets[0].Key {
						t.Fatal("key lost")
					}
				} else if r.Kind != Database || r.Key != "" {
					t.Fatal("wrong database")
				}
				if !bytes.Equal(p, expected) {
					t.Fatalf("payload=%q", p)
				}
				calls++
				return err
			})
			if err != nil || calls != want || !reflect.DeepEqual(got, h) {
				t.Fatalf("got=%+v calls=%d err=%v", got, calls, err)
			}
		})
	}
}

type fragmentReader struct{ *bytes.Reader }

func (r *fragmentReader) Read(p []byte) (int, error) {
	if len(p) > 1 {
		p = p[:1]
	}
	return r.Reader.Read(p)
}
func TestCodecDecodePayloadRefusal(t *testing.T) {
	db := []byte("db")
	a := Asset{strings.Repeat("a", 64), 1, digest([]byte("a"))}
	h := rawHeader(db, []Asset{a})
	cases := []struct {
		name     string
		p        []byte
		early    bool
		maxCalls int
	}{{"short", []byte("d"), false, 1}, {"digest", []byte("xxa"), false, 1}, {"trailing", []byte("dba!"), false, 2}, {"asset_digest", []byte("dbx"), false, 2}, {"early", []byte("dba"), true, 1}}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			calls := 0
			got, err := Decode(context.Background(), bytes.NewReader(envelope(h, c.p)), func(_ Record, r io.Reader) error {
				calls++
				if c.early {
					return nil
				}
				_, e := io.Copy(io.Discard, r)
				return e
			})
			if err == nil || calls != c.maxCalls || got.Database.Bytes != 0 {
				t.Fatalf("calls=%d got=%+v err=%v", calls, got, err)
			}
		})
	}
}
func TestCodecEncodeRefusal(t *testing.T) {
	db := []byte("db")
	h := Header{File{2, digest(db)}, []Asset{}}
	for _, p := range []string{"d", "xx", "db!"} {
		t.Run(p, func(t *testing.T) {
			released := 0
			err := Encode(context.Background(), io.Discard, h, func(Record) (io.Reader, func() error, error) {
				return strings.NewReader(p), func() error { released++; return nil }, nil
			})
			if !errors.Is(err, ErrInvalid) || released != 1 {
				t.Fatalf("released=%d err=%v", released, err)
			}
		})
	}
}

func TestCodecCanonicalAndHeaderCopy(t *testing.T) {
	h := Header{File{2, digest([]byte("db"))}, []Asset{{strings.Repeat("a", 64), 1, digest([]byte("a"))}}}
	expectedHeader := `{"database":{"bytes":2,"sha256":"` + digest([]byte("db")) + `"},"assets":[{"key":"` + strings.Repeat("a", 64) + `","bytes":1,"sha256":"` + digest([]byte("a")) + `"}]}`
	var out bytes.Buffer
	calls := 0
	released := 0
	err := Encode(context.Background(), &out, h, func(r Record) (io.Reader, func() error, error) {
		calls++
		if calls == 1 {
			h.Assets[0] = Asset{Key: "bad"}
			return strings.NewReader("db"), func() error { released++; return nil }, nil
		}
		if released != 1 || r.Kind != Image || r.Key != strings.Repeat("a", 64) || r.Bytes != 1 || r.SHA256 != digest([]byte("a")) {
			t.Fatalf("record not copied/release incomplete: %+v release=%d", r, released)
		}
		return strings.NewReader("a"), func() error { released++; return nil }, nil
	})
	if err != nil || !bytes.Equal(out.Bytes(), envelope(expectedHeader, []byte("dba"))) || released != 2 {
		t.Fatalf("canonical/copy err=%v release=%d output=%q", err, released, out.Bytes())
	}
}
func TestCodecReaderWriterAndCallbackErrors(t *testing.T) {
	for _, kind := range []string{"read", "callback", "short_write", "write", "zero_read", "masked_read"} {
		t.Run(kind, func(t *testing.T) {
			calls := 0
			release := 0
			var err error
			switch kind {
			case "read", "zero_read":
				var r io.Reader = &canaryReader{}
				if kind == "zero_read" {
					r = &zeroReader{}
				}
				err = Encode(context.Background(), io.Discard, smallHeader(), func(Record) (io.Reader, func() error, error) {
					calls++
					return r, func() error { release++; return nil }, nil
				})
			case "write", "short_write":
				var w io.Writer = &canaryWriter{}
				if kind == "short_write" {
					w = &shortWriter{}
				}
				err = Encode(context.Background(), w, smallHeader(), func(Record) (io.Reader, func() error, error) {
					calls++
					return strings.NewReader("db"), func() error { release++; return nil }, nil
				})
			case "callback":
				_, err = Decode(context.Background(), bytes.NewReader(envelope(rawHeader([]byte("db"), nil), []byte("db"))), func(Record, io.Reader) error { calls++; return errors.New("callback CANARY") })
			case "masked_read":
				good := envelope(rawHeader([]byte("db"), nil), nil)
				_, err = Decode(context.Background(), io.MultiReader(bytes.NewReader(good), &canaryReader{}), func(_ Record, r io.Reader) error { calls++; var b [2]byte; _, _ = r.Read(b[:]); return nil })
			}
			if !errors.Is(err, ErrIO) || strings.Contains(err.Error(), "CANARY") {
				t.Fatalf("unsafe err=%v", err)
			}
			if kind == "write" || kind == "short_write" {
				if calls != 0 || release != 0 {
					t.Fatalf("source opened after output error")
				}
			} else if calls != 1 {
				t.Fatalf("calls=%d", calls)
			}
			if kind == "read" || kind == "zero_read" {
				if release != 1 {
					t.Fatalf("release=%d", release)
				}
			}
		})
	}
}

type canaryReader struct{}

func (*canaryReader) Read([]byte) (int, error) { return 0, errors.New("read CANARY") }

type zeroReader struct{}

func (*zeroReader) Read([]byte) (int, error) { return 0, nil }

type canaryWriter struct{}

func (*canaryWriter) Write([]byte) (int, error) { return 0, errors.New("write CANARY") }

type shortWriter struct{}

func (*shortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }
func TestCodecDecodeCallbackBoundary(t *testing.T) {
	h := rawHeader([]byte("db"), []Asset{{strings.Repeat("a", 64), 1, digest([]byte("a"))}})
	calls := 0
	_, err := Decode(context.Background(), bytes.NewReader(envelope(h, []byte("dba"))), func(r Record, in io.Reader) error {
		calls++
		p, e := io.ReadAll(in)
		if r.Kind == Database && string(p) != "db" {
			t.Fatalf("callback leaked next record: %q", p)
		}
		if r.Kind == Image && string(p) != "a" {
			t.Fatalf("image=%q", p)
		}
		return e
	})
	if err != nil || calls != 2 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestCodecFixedScratchAndPayloadWriteFailure(t *testing.T) {
	payload := bytes.Repeat([]byte("x"), 8193)
	h := Header{File{int64(len(payload)), digest(payload)}, []Asset{}}
	source := &boundedRead{Reader: bytes.NewReader(payload)}
	release := 0
	if err := Encode(context.Background(), io.Discard, h, func(Record) (io.Reader, func() error, error) {
		return source, func() error { release++; return nil }, nil
	}); err != nil || source.maximum > 4096 || source.maximum < 1 || release != 1 {
		t.Fatalf("maximum=%d release=%d err=%v", source.maximum, release, err)
	}
	source = &boundedRead{Reader: bytes.NewReader(payload)}
	release = 0
	w := &payloadFailWriter{}
	err := Encode(context.Background(), w, h, func(Record) (io.Reader, func() error, error) {
		return source, func() error { release++; return nil }, nil
	})
	if !errors.Is(err, ErrIO) || release != 1 || source.reads != 1 || strings.Contains(err.Error(), "CANARY") {
		t.Fatalf("reads=%d release=%d err=%v", source.reads, release, err)
	}
}

type boundedRead struct {
	*bytes.Reader
	maximum, reads int
}

func (r *boundedRead) Read(p []byte) (int, error) {
	r.reads++
	if len(p) > r.maximum {
		r.maximum = len(p)
	}
	return r.Reader.Read(p)
}

type payloadFailWriter struct{ writes int }

func (w *payloadFailWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes > 2 {
		return 0, errors.New("CANARY")
	}
	return len(p), nil
}
