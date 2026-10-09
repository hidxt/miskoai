package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// These tests catch loss of zero-byte evidence, cursor advancement on quarantine,
// unsafe row admission, and counting bytes instead of frames for capacity.
func TestEmptyPollEvidenceLifecycle(t *testing.T) {
	for _, variant := range []struct {
		name string
		body []byte
	}{{"nil", nil}, {"empty", []byte{}}} {
		t.Run(variant.name, func(t *testing.T) {
			s, path := testStore(t)
			ctx := context.Background()
			scope := Scope{"synthetic-account", "synthetic-user"}
			if _, err := s.db.Exec(`INSERT INTO channel_cursors(account,user,cursor) VALUES(?,?,'synthetic-prior')`, scope.Account, scope.User); err != nil {
				t.Fatal(err)
			}
			id, err := s.RecordPoll(ctx, scope, "synthetic-prior", variant.body)
			if err != nil {
				t.Fatalf("record zero-byte response: %v", err)
			}
			assertEmptyPoll(t, s, scope, id, "pending")
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			s, err = Open(path)
			if err != nil {
				t.Fatalf("reopen zero-byte evidence: %v", err)
			}
			defer s.Close()
			assertEmptyPoll(t, s, scope, id, "pending")
			// Empty malformed data is quarantined; ResolvePoll is never called.
			if err := s.QuarantinePoll(ctx, scope, id); err != nil {
				t.Fatal(err)
			}
			assertEmptyPoll(t, s, scope, id, "quarantined")
			for _, candidate := range []Scope{scope, {scope.Account, "another-user"}} {
				if _, err := s.RecordPoll(ctx, candidate, "synthetic-prior", []byte("new")); !errors.Is(err, ErrCapacity) {
					t.Fatalf("quarantine must block account: %v", err)
				}
			}
			backup := filepath.Join(filepath.Dir(path), "empty-backup.db")
			if err := s.Backup(ctx, backup); err != nil {
				t.Fatal(err)
			}
			if err := ValidateBackup(ctx, backup); err != nil {
				t.Fatalf("validate schema3 empty evidence backup: %v", err)
			}
			// Restore the validated completed snapshot to a fresh private child.
			dir := filepath.Join(t.TempDir(), "private-restored")
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(backup)
			if err != nil {
				t.Fatal(err)
			}
			restoredPath := filepath.Join(dir, "memory.db")
			if err := os.WriteFile(restoredPath, data, 0600); err != nil {
				t.Fatal(err)
			}
			restored, err := Open(restoredPath)
			if err != nil {
				t.Fatalf("open restored snapshot: %v", err)
			}
			defer restored.Close()
			assertEmptyPoll(t, restored, scope, id, "quarantined")
			var version int
			if err := restored.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 3 {
				t.Fatalf("schema version=%d error=%v", version, err)
			}
		})
	}
}

func assertEmptyPoll(t *testing.T, s *Store, scope Scope, id int64, state string) {
	t.Helper()
	ctx := context.Background()
	f, err := s.PendingPoll(ctx, scope)
	if err != nil || f == nil {
		t.Fatalf("read zero-byte frame: frame present=%v error=%v", f != nil, err)
	}
	if f.ID != id || f.Scope != scope || f.Cursor != "synthetic-prior" || len(f.Body) != 0 || f.State != state {
		t.Fatalf("evidence metadata/empty body mismatch")
	}
	var kind string
	var size int
	if err := s.db.QueryRow(`SELECT typeof(body),length(body) FROM poll_frames WHERE id=?`, id).Scan(&kind, &size); err != nil || kind != "blob" || size != 0 {
		t.Fatalf("stored body type=%s bytes=%d error=%v", kind, size, err)
	}
	if cursor, err := s.Cursor(ctx, scope); err != nil || cursor != "synthetic-prior" {
		t.Fatalf("cursor advanced: %q %v", cursor, err)
	}
}

func TestEmptyPollFrameCapacity(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		if _, err := s.RecordPoll(ctx, Scope{fmt.Sprintf("synthetic-%d", i), "user"}, "", nil); err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
	}
	if _, err := s.RecordPoll(ctx, Scope{"synthetic-fifth", "user"}, "", []byte{}); !errors.Is(err, ErrCapacity) {
		t.Fatalf("fifth empty frame: %v", err)
	}
}

func TestEmptyPollSchema2SnapshotAdmission(t *testing.T) {
	s, path := testSchema2Store(t)
	ctx := context.Background()
	if _, err := s.db.Exec(`INSERT INTO poll_frames(account,user,cursor,body,state) VALUES('synthetic','user','',X'','quarantined')`); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(filepath.Dir(path), "snapshot.db")
	if err := s.Backup(ctx, backup); err != nil {
		t.Fatal(err)
	}
	if err := ValidateBackup(ctx, backup); err != nil {
		t.Fatalf("schema2 zero-byte BLOB snapshot admission: %v", err)
	}
	reopened, err := Open(backup)
	if err != nil {
		t.Fatalf("schema2 zero-byte BLOB reopen admission: %v", err)
	}
	defer reopened.Close()
	f, err := reopened.PendingPoll(ctx, Scope{"synthetic", "user"})
	if err != nil || f == nil || len(f.Body) != 0 || f.State != "quarantined" {
		t.Fatalf("snapshot evidence lost: present=%v error=%v", f != nil, err)
	}
}

func TestEmptyPollKeepsBodyBoundsAndTypes(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	scope := Scope{"synthetic-account", "user"}
	if _, err := s.RecordPoll(ctx, scope, "", make([]byte, (2<<20)+1)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("oversized body: %v", err)
	}
	for _, body := range []any{"", int64(0), nil, make([]byte, (2<<20)+1)} {
		_, err := s.db.Exec(`INSERT INTO poll_frames(account,user,cursor,body,state) VALUES(?,?,'',?,'pending')`, scope.Account, scope.User, body)
		if body == nil {
			if err == nil || err.Error() != "constraint failed: NOT NULL constraint failed: poll_frames.body (1299)" {
				t.Fatalf("SQL NULL must fail the body NOT NULL constraint: %v", err)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if frame, err := s.PendingPoll(ctx, scope); !errors.Is(err, ErrInvalid) || frame != nil {
			t.Fatalf("unsafe body admitted: %T %v", body, err)
		}
		if _, err := s.db.Exec(`DELETE FROM poll_frames`); err != nil {
			t.Fatal(err)
		}
	}
}
