package maintenance

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/hidxt/miskoai/internal/privatefs"
	"io"
	"os"
	"path/filepath"
	"unicode/utf8"
)

const pauseName = ".miskoai-restore-pause.json"
const pausePayload = `{"version":1,"reason":"restore_requires_reconciliation"}`

var errPause = errors.New("restore pause marker is unsafe or invalid; operator reconciliation required")

// RestorePaused strictly checks the durable restore safety marker.
func RestorePaused(dataDir string) (bool, error) {
	dir, e := privatefs.Resolve(dataDir)
	if e != nil {
		return false, errPause
	}
	if privatefs.CheckDir(dir) != nil {
		return false, errPause
	}
	path := filepath.Join(dir, pauseName)
	if _, e = os.Lstat(path); errors.Is(e, os.ErrNotExist) {
		return false, nil
	} else if e != nil {
		return false, errPause
	}
	b, e := privatefs.Read(path, 1024)
	if e != nil || !utf8.Valid(b) {
		return false, errPause
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	tok, e := dec.Token()
	if e != nil || tok != json.Delim('{') {
		return false, errPause
	}
	seen := map[string]bool{}
	for dec.More() {
		t, e := dec.Token()
		k, ok := t.(string)
		if e != nil || !ok || seen[k] {
			return false, errPause
		}
		seen[k] = true
		switch k {
		case "version":
			v, ve := dec.Token()
			n, ok := v.(json.Number)
			if ve != nil || !ok || n.String() != "1" {
				return false, errPause
			}
		case "reason":
			var v string
			if dec.Decode(&v) != nil || v != "restore_requires_reconciliation" {
				return false, errPause
			}
		default:
			return false, errPause
		}
	}
	tok, e = dec.Token()
	if e != nil || tok != json.Delim('}') || len(seen) != 2 {
		return false, errPause
	}
	if _, e = dec.Token(); e != io.EOF {
		return false, errPause
	}
	return true, nil
}

type durableFile interface {
	Write([]byte) (int, error)
	Sync() error
	Close() error
}

func ensurePause(l *Lock) error {
	return ensurePauseWith(l, func(path string) (durableFile, error) { return privatefs.Create(path) })
}
func ensurePauseWith(l *Lock, create func(string) (durableFile, error)) error {
	if e := l.verifyLocked(); e != nil {
		return e
	}
	paused, e := RestorePaused(l.dir)
	if e != nil {
		return e
	}
	if paused {
		return syncPause(l)
	}
	f, e := create(filepath.Join(l.dir, pauseName))
	if e != nil {
		return errPause
	}
	n, we := f.Write([]byte(pausePayload))
	se := f.Sync()
	ce := f.Close()
	if n != len(pausePayload) || we != nil || se != nil || ce != nil {
		return errPause
	}
	paused, e = RestorePaused(l.dir)
	if e != nil || !paused {
		return errPause
	}
	return nil
}

// Re-sync a valid marker on retries, including a prior failed file sync.
func syncPause(l *Lock) error {
	p := filepath.Join(l.dir, pauseName)
	if privatefs.CheckFile(p, 1024) != nil {
		return errPause
	}
	before, e := os.Lstat(p)
	if e != nil {
		return errPause
	}
	f, e := os.OpenFile(p, os.O_RDWR, 0)
	if e != nil {
		return errPause
	}
	actual, e := f.Stat()
	if e != nil || !sameMetadata(before, actual) {
		f.Close()
		return errPause
	}
	se := f.Sync()
	ce := f.Close()
	after, e := os.Lstat(p)
	if se != nil || ce != nil || e != nil || !sameMetadata(actual, after) || privatefs.CheckFile(p, 1024) != nil {
		return errPause
	}
	return nil
}

// ClearRestorePause is only for explicit operator reconciliation with ownership.
func ClearRestorePause(l *Lock) error {
	if l == nil {
		return errOwnership
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if e := l.verifyLocked(); e != nil {
		return e
	}
	paused, e := RestorePaused(l.dir)
	if e != nil || !paused {
		return e
	}
	p := filepath.Join(l.dir, pauseName)
	info, e := os.Lstat(p)
	if e != nil {
		return errPause
	}
	if e = l.verifyLocked(); e != nil {
		return e
	}
	if _, e = RestorePaused(l.dir); e != nil {
		return e
	}
	if e = removeOwned(p, info); e != nil {
		return e
	}
	if syncDirectory(l.dir) != nil {
		return errPause
	}
	return nil
}
