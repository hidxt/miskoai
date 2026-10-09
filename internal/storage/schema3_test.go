package storage

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSchema3FreshManifest(t *testing.T) {
	s, _ := testStore(t)
	var v int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != 3 {
		t.Fatalf("memory/profile storage requires schema3, got %d", v)
	}
	if err := validateManifest(context.Background(), s.db); err != nil {
		t.Fatal(err)
	}
}

// Legacy tests build exactly schema2 instead of silently following Open's
// latest migration. No fixture is an actual database or private authorization.
func testSchema2Store(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "private", "schema2.db")
	if e := os.Mkdir(filepath.Dir(path), 0700); e != nil {
		t.Fatal(e)
	}
	db, e := sql.Open("sqlite", path)
	if e != nil {
		t.Fatal(e)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, e = db.Exec(schema + schema2 + "PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000; PRAGMA wal_checkpoint(TRUNCATE)"); e != nil {
		db.Close()
		t.Fatal(e)
	}
	s := &Store{db: db}
	t.Cleanup(func() { s.Close() })
	return s, path
}

func TestSchema2SequenceAdmissionRefusesUnsafeMetadataWithoutMutation(t *testing.T) {
	cases := []struct {
		name, query string
		args        []any
	}{
		{"unknown-name", `UPDATE sqlite_sequence SET name='foreign' WHERE name='poll_frames'`, nil},
		{"schema3-name", `UPDATE sqlite_sequence SET name='candidates' WHERE name='poll_frames'`, nil},
		{"oversized-name", `UPDATE sqlite_sequence SET name=? WHERE name='poll_frames'`, []any{strings.Repeat("x", 65)}},
		{"blob-name", `UPDATE sqlite_sequence SET name=X'706F6C6C5F6672616D6573' WHERE name='poll_frames'`, nil},
		{"blob-sequence", `UPDATE sqlite_sequence SET seq=X'31' WHERE name='poll_frames'`, nil},
		{"negative-sequence", `UPDATE sqlite_sequence SET seq=-1 WHERE name='poll_frames'`, nil},
		{"duplicate-sequence", `DELETE FROM sqlite_sequence WHERE name='inbox'; INSERT INTO sqlite_sequence(name,seq) VALUES('poll_frames',5)`, nil},
		{"over-row-count", `INSERT INTO sqlite_sequence(name,seq) VALUES('inbox',7)`, nil},
		{"poll-sequence-below-row", `UPDATE sqlite_sequence SET seq=4 WHERE name='poll_frames'`, nil},
		{"inbox-sequence-below-row", `UPDATE sqlite_sequence SET seq=6 WHERE name='inbox'`, nil},
		{"missing-poll-sequence", `DELETE FROM sqlite_sequence WHERE name='poll_frames'`, nil},
		{"missing-inbox-sequence", `DELETE FROM sqlite_sequence WHERE name='inbox'`, nil},
	}
	for _, tc := range cases {
		for _, wal := range []bool{false, true} {
			mode := "snapshot"
			if wal {
				mode = "wal"
			}
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				s, path := testSchema2Store(t)
				if _, e := s.db.Exec(`INSERT INTO poll_frames(id,account,user,cursor,body,state) VALUES(5,'a','u','',X'','pending'); INSERT INTO inbox(sequence,account,user,message_id,text,context_token,received_at) VALUES(7,'a','u','m','synthetic','',1)`); e != nil {
					t.Fatal(e)
				}
				if _, e := s.db.Exec(tc.query, tc.args...); e != nil {
					t.Fatal(e)
				}
				if !wal {
					if e := s.Close(); e != nil {
						t.Fatal(e)
					}
				}
				before, e := os.ReadFile(path)
				if e != nil {
					t.Fatal(e)
				}
				var wb []byte
				if wal {
					wb, e = os.ReadFile(path + "-wal")
					if e != nil {
						t.Fatal(e)
					}
				}
				opened, e := Open(path)
				if opened != nil {
					opened.Close()
				}
				if !errors.Is(e, ErrInvalid) {
					t.Errorf("unsafe schema2 sequence admitted: %v", e)
				}
				after, e := os.ReadFile(path)
				if e != nil || !bytes.Equal(before, after) {
					t.Errorf("refused source main mutated: %v", e)
				}
				if wal {
					wa, e := os.ReadFile(path + "-wal")
					if e != nil || !bytes.Equal(wb, wa) {
						t.Errorf("refused source WAL mutated: %v", e)
					}
				} else {
					for _, suffix := range []string{"-wal", "-shm", "-journal"} {
						if _, e := os.Stat(path + suffix); !os.IsNotExist(e) {
							t.Errorf("unexpected sidecar %s: %v", suffix, e)
						}
					}
				}
			})
		}
	}
}

func TestSchema3UnsafeSnapshotAndWALAreRefusedWithoutMutation(t *testing.T) {
	cases := []struct {
		name, query string
		args        []any
		want        error
	}{
		{"sequence-metadata", `INSERT INTO sqlite_sequence(name,seq) VALUES(?,1)`, []any{strings.Repeat("x", 65)}, ErrInvalid},
		{"sequence-negative", `INSERT INTO sqlite_sequence(name,seq) VALUES('candidates',-1)`, nil, ErrInvalid},
		{"summary-shape", `INSERT INTO derived VALUES('a','u',?,0,1,1)`, []any{[]byte("blob")}, ErrInvalid},
		{"summary-bytes", `INSERT INTO derived VALUES('a','u',?,0,1,1)`, []any{strings.Repeat("界", 2731)}, ErrInvalid},
		{"summary-utf8", `INSERT INTO derived VALUES('a','u',CAST(? AS TEXT),0,1,1)`, []any{[]byte{0xff}}, ErrInvalid},
		{"summary-control", `INSERT INTO derived VALUES('a','u',?,0,1,1)`, []any{"x\x01"}, ErrInvalid},
		{"summary-revision", `INSERT INTO derived VALUES('a','u','',0,-1,1)`, nil, ErrInvalid},
		{"summary-watermark", `INSERT INTO derived VALUES('a','u','',2,1,1)`, nil, ErrInvalid},
		{"summary-time", `INSERT INTO derived VALUES('a','u','',0,1,0)`, nil, ErrInvalid},
		{"summary-scopes", `WITH RECURSIVE n(x) AS(VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<17) INSERT INTO derived SELECT 'a',CAST(x AS TEXT),'',0,1,1 FROM n`, nil, ErrCapacity},
		{"candidate-quote", `INSERT INTO candidates(account,user,content,message_id,quote,created_at) VALUES('a','u','x','source','assistant',1)`, nil, ErrInvalid},
		{"candidate-content", `INSERT INTO candidates(account,user,content,message_id,quote,created_at) VALUES('a','u',?,'source','user',1)`, []any{strings.Repeat("x", 1025)}, ErrInvalid},
		{"candidate-quote-bytes", `INSERT INTO candidates(account,user,content,message_id,quote,created_at) VALUES('a','u','x','source',?,1)`, []any{strings.Repeat("x", 1025)}, ErrInvalid},
		{"candidate-id", `INSERT INTO candidates(account,user,content,message_id,quote,created_at) VALUES('a','u','x',?,'user',1)`, []any{strings.Repeat("x", 513)}, ErrInvalid},
		{"candidate-time", `INSERT INTO candidates(account,user,content,message_id,quote,created_at) VALUES('a','u','x','source','user',253402300800000)`, nil, ErrInvalid},
		{"candidate-global", `WITH RECURSIVE n(x) AS(VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<257) INSERT INTO candidates(account,user,content,message_id,quote,created_at) SELECT 'a','u',CAST(x AS TEXT),'source','user',1 FROM n`, nil, ErrCapacity},
		{"candidate-scope", `WITH RECURSIVE n(x) AS(VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<129) INSERT INTO candidates(account,user,content,message_id,quote,created_at) SELECT 'a','u',CAST(x AS TEXT),'source','user',1 FROM n`, nil, ErrCapacity},
		{"profile-body", `INSERT INTO profiles VALUES('a','u','p',?)`, []any{strings.Repeat("x", 8193)}, ErrInvalid},
		{"profile-canonical", `INSERT INTO profiles VALUES('a','u','p','{"id":"p"}')`, nil, ErrInvalid},
		{"profile-shape", `INSERT INTO profiles VALUES('a','u','p',?)`, []any{[]byte("blob")}, ErrInvalid},
		{"selection-foreign", `INSERT INTO profile_selections VALUES('a','u','missing')`, nil, ErrInvalid},
		{"selection-id", `INSERT INTO profile_selections VALUES('a','u','UPPER')`, nil, ErrInvalid},
		{"selection-scopes", `WITH RECURSIVE n(x) AS(VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<17) INSERT INTO profile_selections SELECT 'a',CAST(x AS TEXT),'warm' FROM n`, nil, ErrCapacity},
	}
	for _, tc := range cases {
		for _, wal := range []bool{false, true} {
			mode := "snapshot"
			if wal {
				mode = "wal"
			}
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				s, path := testStore(t)
				ctx := context.Background()
				sentFixture(t, s, Scope{"a", "u"}, "source", "user evidence", "assistant")
				if _, e := s.db.Exec(tc.query, tc.args...); e != nil {
					t.Fatal(e)
				}
				if !wal {
					if e := s.Close(); e != nil {
						t.Fatal(e)
					}
				}
				before, e := os.ReadFile(path)
				if e != nil {
					t.Fatal(e)
				}
				var wb []byte
				if wal {
					wb, e = os.ReadFile(path + "-wal")
					if e != nil {
						t.Fatal(e)
					}
				}
				if !wal {
					if e = ValidateBackup(ctx, path); !errors.Is(e, tc.want) {
						t.Fatalf("unsafe snapshot accepted want %v got %v", tc.want, e)
					}
				}
				opened, e := Open(path)
				if opened != nil {
					opened.Close()
				}
				if !errors.Is(e, tc.want) {
					t.Fatalf("unsafe candidate admitted want %v got %v", tc.want, e)
				}
				after, e := os.ReadFile(path)
				if e != nil || !bytes.Equal(before, after) {
					t.Fatalf("main mutated %v", e)
				}
				if wal {
					wa, e := os.ReadFile(path + "-wal")
					if e != nil || !bytes.Equal(wb, wa) {
						t.Fatalf("WAL mutated %v", e)
					}
				} else {
					for _, suffix := range []string{"-wal", "-shm", "-journal"} {
						if _, e := os.Stat(path + suffix); !os.IsNotExist(e) {
							t.Fatalf("created %s: %v", suffix, e)
						}
					}
				}
			})
		}
	}
}

func TestExactOldManifestsMigratePreservingData(t *testing.T) {
	for _, v := range []int{1, 2} {
		t.Run(string(rune('0'+v)), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "private", "old.db")
			if e := os.Mkdir(filepath.Dir(path), 0700); e != nil {
				t.Fatal(e)
			}
			db, e := sql.Open("sqlite", path)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = db.Exec(schema); e != nil {
				t.Fatal(e)
			}
			if v == 2 {
				if _, e = db.Exec(schema2); e != nil {
					t.Fatal(e)
				}
			}
			if _, e = db.Exec(`INSERT INTO messages VALUES('a','u','old','user','reply','sent',1,1)`); e != nil {
				t.Fatal(e)
			}
			if e = db.Close(); e != nil {
				t.Fatal(e)
			}
			if e = ValidateBackup(context.Background(), path); e != nil {
				t.Fatal(e)
			}
			s, e := Open(path)
			if e != nil {
				t.Fatal(e)
			}
			defer s.Close()
			d, n, h, e := s.DerivedHistory(context.Background(), Scope{"a", "u"}, 16)
			if e != nil || d.Revision != 0 || n != 1 || len(h) != 2 {
				t.Fatalf("old data %v %d %v %v", d, n, h, e)
			}
		})
	}
}

func TestSchema2MainSchema3WALAdmissionAndCheckpoint(t *testing.T) {
	for _, unsafe := range []bool{false, true} {
		t.Run(fmt.Sprint(unsafe), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "private", "upgrade.db")
			if e := os.Mkdir(filepath.Dir(path), 0700); e != nil {
				t.Fatal(e)
			}
			db, e := sql.Open("sqlite", path)
			if e != nil {
				t.Fatal(e)
			}
			defer db.Close()
			if _, e = db.Exec(schema + schema2 + "PRAGMA journal_mode=WAL; PRAGMA wal_checkpoint(TRUNCATE)"); e != nil {
				t.Fatal(e)
			}
			if _, e = db.Exec(schema3); e != nil {
				t.Fatal(e)
			}
			if unsafe {
				if _, e = db.Exec(`INSERT INTO derived VALUES('a','u',X'00',0,1,1)`); e != nil {
					t.Fatal(e)
				}
			}
			before, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			wb, e := os.ReadFile(path + "-wal")
			if e != nil {
				t.Fatal(e)
			}
			s, e := Open(path)
			if unsafe {
				if s != nil {
					s.Close()
				}
				if !errors.Is(e, ErrInvalid) {
					t.Fatalf("unsafe effective3 admitted %v", e)
				}
				after, e := os.ReadFile(path)
				if e != nil || !bytes.Equal(before, after) {
					t.Fatalf("main mutated %v", e)
				}
				wa, e := os.ReadFile(path + "-wal")
				if e != nil || !bytes.Equal(wb, wa) {
					t.Fatalf("wal mutated %v", e)
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			defer s.Close()
			uriPath := filepath.ToSlash(path)
			if filepath.VolumeName(path) != "" {
				uriPath = "/" + uriPath
			}
			dsn := url.URL{Scheme: "file", Path: uriPath, RawQuery: "mode=ro&immutable=1"}
			probe, e := sql.Open("sqlite", dsn.String())
			if e != nil {
				t.Fatal(e)
			}
			defer probe.Close()
			var v int
			if e = probe.QueryRow("PRAGMA user_version").Scan(&v); e != nil || v != 3 {
				t.Fatalf("upgrade main not checkpointed %d %v", v, e)
			}
		})
	}
}
