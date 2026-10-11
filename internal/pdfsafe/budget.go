package pdfsafe

import (
	"context"
	"errors"
	"io"
	"time"
)

const (
	maxInput            = 4 << 20
	maxNodes            = 32768
	maxRevisions        = 32
	maxDepth            = 64
	maxToken            = 64 << 10
	maxRetained         = 8 << 20
	maxDecoded          = 8 << 20
	maxWork             = 32 << 20
	maxFilters          = 4
	maxRow              = 4 << 10
	maxStreams          = 256
	maxOperands         = 256
	maxDictScopes       = 64
	maxPages            = 64
	maxOutput           = 128 << 10
	maxCMapEntries      = 8192
	maxCodeBytes        = 4
	maxReplacementBytes = 1024
)

var (
	errLimit       = errors.New("PDF resource limit")
	errMalformed   = errors.New("malformed PDF")
	errUnsupported = errors.New("unsupported PDF")
)

// fault carries only a fixed safe error. It crosses private helpers and is
// caught by the outer parse/extraction boundary, never by the interpreter.
type fault struct{ err error }

func fail(err error) { panic(fault{err}) }

// budget belongs to one actual synchronous parse/extraction lifetime. Counters
// are cumulative, including work and retained payload from repeated lookups.
type budget struct {
	ctx                                              context.Context
	deadline                                         time.Time
	work, nodes, retained, decoded, revisions, depth int64
	streams                                          map[*streamReader]struct{}
	objects                                          map[objptr]bool
	offsets                                          map[int64]bool
	xrefs                                            map[int64]bool
}

func newBudget(ctx context.Context) *budget {
	if ctx == nil {
		ctx = context.Background()
	}
	b := &budget{ctx: ctx, deadline: time.Now().Add(2 * time.Second)}
	b.check()
	b.retain(256)
	b.streams = make(map[*streamReader]struct{})
	b.objects = make(map[objptr]bool)
	b.offsets = make(map[int64]bool)
	b.xrefs = make(map[int64]bool)
	return b
}
func (b *budget) check() {
	if err := b.ctx.Err(); err != nil {
		fail(err)
	}
	if !time.Now().Before(b.deadline) {
		fail(context.DeadlineExceeded)
	}
}
func (b *budget) charge(n int64) {
	b.check()
	if n < 0 || n > maxWork-b.work {
		fail(errLimit)
	}
	b.work += n
}
func (b *budget) retain(n int64) {
	b.charge(1)
	if n < 0 || n > maxRetained-b.retained {
		fail(errLimit)
	}
	b.retained += n
}
func (b *budget) node(n int64) {
	b.charge(n)
	if n < 0 || n > maxNodes-b.nodes {
		fail(errLimit)
	}
	b.nodes += n
}
func (b *budget) enter() {
	b.charge(1)
	if b.depth >= maxDepth {
		fail(errLimit)
	}
	b.depth++
}
func (b *budget) leave() { b.depth-- }
func (b *budget) revision(off, size int64) {
	b.charge(1)
	if off < 0 || off >= size {
		fail(errMalformed)
	}
	if b.xrefs[off] {
		fail(errMalformed)
	}
	if b.revisions >= maxRevisions {
		fail(errLimit)
	}
	b.revisions++
	b.node(1)
	b.retain(16)
	b.xrefs[off] = true
}
func (b *budget) closeStreams() error {
	var err error
	for s := range b.streams {
		if s.Close() != nil {
			err = errMalformed
		}
	}
	return err
}

type checkedReaderAt struct {
	r    io.ReaderAt
	b    *budget
	size int64
}

func (r *checkedReaderAt) ReadAt(p []byte, off int64) (int, error) {
	r.b.charge(int64(len(p)) + 1)
	if off < 0 || off > r.size {
		fail(errMalformed)
	}
	if int64(len(p)) > r.size-off {
		p = p[:r.size-off]
	}
	if len(p) == 0 {
		return 0, io.EOF
	}
	n, e := r.r.ReadAt(p, off)
	r.b.check()
	if n < 0 || n > len(p) {
		fail(errMalformed)
	}
	if e != nil && e != io.EOF {
		fail(errMalformed)
	}
	if n == 0 && e == nil {
		fail(errMalformed)
	}
	if n < len(p) && e == nil {
		fail(errMalformed)
	}
	return n, e
}
