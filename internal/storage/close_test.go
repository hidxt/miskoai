package storage

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	sqlite "modernc.org/sqlite"
)

type closeConnector struct {
	path                     string
	entered, release, closed chan struct{}
	once                     sync.Once
	closeError               bool
}

func (c *closeConnector) Driver() driver.Driver { return &sqlite.Driver{} }
func (c *closeConnector) Connect(context.Context) (driver.Conn, error) {
	v, e := c.Driver().Open(c.path)
	if e != nil {
		return nil, e
	}
	return &closeConn{Conn: v, c: c}, nil
}

type closeConn struct {
	driver.Conn
	c *closeConnector
}

func (c *closeConn) Begin() (driver.Tx, error) {
	v, e := c.Conn.Begin()
	if e != nil {
		return nil, e
	}
	return &closeTx{Tx: v, c: c.c}, nil
}
func (c *closeConn) Close() error {
	e := c.Conn.Close()
	c.c.once.Do(func() { close(c.c.closed) })
	if c.c.closeError {
		return errors.New("synthetic driver close refusal")
	}
	return e
}

type closeTx struct {
	driver.Tx
	c *closeConnector
}

func (t *closeTx) Rollback() error { close(t.c.entered); <-t.c.release; return t.Tx.Rollback() }
func closeFixture(t *testing.T) (*Store, *closeConnector) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "private")
	if e := os.Mkdir(dir, 0700); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(dir, "synthetic.db")
	s, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	c := &closeConnector{path: path, entered: make(chan struct{}), release: make(chan struct{}), closed: make(chan struct{})}
	db := sql.OpenDB(c)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	return &Store{db: db}, c
}
func TestStoreCloseJoinsCanceledRollbackAndConcurrentCallers(t *testing.T) {
	s, c := closeFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	var version int
	if e = tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); e != nil {
		t.Fatal(e)
	}
	var release sync.Once
	t.Cleanup(func() { release.Do(func() { close(c.release) }); _ = s.db.Close() })
	cancel()
	select {
	case <-c.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("real rollback not entered")
	}
	if e = tx.Rollback(); !errors.Is(e, sql.ErrTxDone) {
		t.Fatalf("caller rollback=%v", e)
	}
	done := make(chan error, 2)
	go func() { done <- s.Close() }()
	go func() { done <- s.Close() }()
	watchdog := time.NewTimer(2 * time.Second)
	defer watchdog.Stop()
	// Wait for a concrete database/sql capacity waiter, not a scheduling delay.
	for s.db.Stats().WaitCount == 0 {
		select {
		case e = <-done:
			t.Fatalf("Close returned before canceled rollback/driver cleanup: %v", e)
		case <-watchdog.C:
			t.Fatal("Close did not enter connection drain")
		default:
			runtime.Gosched()
		}
	}
	select {
	case e = <-done:
		t.Fatalf("concurrent Close returned before rollback release: %v", e)
	default:
	}
	release.Do(func() { close(c.release) })
	for i := 0; i < 2; i++ {
		select {
		case e = <-done:
			if e != nil {
				t.Fatal(e)
			}
		case <-watchdog.C:
			t.Fatal("Close did not join after release")
		}
	}
	select {
	case <-c.closed:
	default:
		t.Fatal("successful Close preceded real driver cleanup")
	}
	if e = s.Close(); e != nil {
		t.Fatalf("repeated Close=%v", e)
	}
}
func TestStoreClosePreservesIdleDriverCloseError(t *testing.T) {
	s, c := closeFixture(t)
	c.closeError = true
	if e := s.db.Ping(); e != nil {
		t.Fatal(e)
	}
	first := s.Close()
	if !errors.Is(first, ErrStorage) {
		t.Fatalf("safe close refusal=%v", first)
	}
	if next := s.Close(); next != first {
		t.Fatalf("close error not cached: %v then %v", first, next)
	}
}
func TestStoreCloseRefusesUnjoinedCancellation(t *testing.T) {
	closeCtx, closeCancel := context.WithCancel(context.Background())
	closeCancel()
	storeCloseRefusesUnjoined(t, closeCtx)
}
func TestStoreCloseRefusesUnjoinedDeadline(t *testing.T) {
	closeCtx, closeCancel := context.WithDeadline(context.Background(), time.Unix(0, 0))
	defer closeCancel()
	storeCloseRefusesUnjoined(t, closeCtx)
}
func storeCloseRefusesUnjoined(t *testing.T, closeCtx context.Context) {
	t.Helper()
	s, c := closeFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	var release sync.Once
	t.Cleanup(func() { release.Do(func() { close(c.release) }); _ = s.db.Close() })
	cancel()
	select {
	case <-c.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("real rollback not entered")
	}
	first := s.closeContext(closeCtx)
	if !errors.Is(first, ErrStorage) {
		t.Fatalf("unjoined close=%v", first)
	}
	if next := s.Close(); next != first {
		t.Fatal("unjoined close refusal not cached")
	}
	if e = tx.Rollback(); !errors.Is(e, sql.ErrTxDone) {
		t.Fatalf("caller rollback=%v", e)
	}
	if e = s.db.Ping(); e == nil {
		t.Fatal("failed close still admits new operations")
	}
	select {
	case <-c.closed:
		t.Fatal("fixture did not hold driver cleanup")
	default:
	}
	release.Do(func() { close(c.release) })
	select {
	case <-c.closed:
	case <-time.After(2 * time.Second):
		t.Fatal("released cleanup did not complete")
	}
}
