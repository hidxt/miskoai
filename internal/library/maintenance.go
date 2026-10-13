package library

import (
	"context"
	"errors"
	"github.com/hidxt/miskoai/internal/storage"
	"os"
)

// Maintenance is an admission-bound synchronous view. Callers release their
// snapshot Store lease before RetainCurrent and must not detach or reenter work.
type Maintenance struct {
	self    *Maintenance
	manager *Manager
	ctx     context.Context
}

func (m *Manager) WithMaintenance(parent context.Context, consume func(context.Context, *Maintenance) error) (err error) {
	if consume == nil {
		return ErrInvalid
	}
	ctx, done, e := m.begin(parent)
	if e != nil {
		return e
	}
	defer done()
	v := &Maintenance{manager: m, ctx: ctx}
	v.self = v
	defer func() { v.self = nil; v.manager = nil; v.ctx = nil; err = safe(errors.Join(err, ctx.Err())) }()
	return consume(ctx, v)
}
func (v *Maintenance) RetainCurrent(rows []storage.LibraryAsset) error {
	if v == nil || v.self != v || v.manager == nil || v.ctx == nil {
		return ErrInvalid
	}
	if e := v.ctx.Err(); e != nil {
		return e
	}
	m := v.manager
	if e := m.checkRetainedStage(); e != nil {
		return e
	}
	if e := validCurrent(rows); e != nil {
		return e
	}
	old, e := m.readRetained()
	if e != nil {
		return e
	}
	union := make(map[string]retainedRecord, len(old.Entries)+len(rows))
	for _, r := range old.Entries {
		a := r.asset()
		if e = m.validate(v.ctx, a, noContent); e != nil {
			return safe(errors.Join(e, ErrRecovery))
		}
		union[r.key()] = r
	}
	for _, a := range rows {
		r := record(a)
		if prior, ok := union[r.key()]; ok && prior != r {
			return ErrRecovery
		}
		if _, e = os.Lstat(m.path(a)); errors.Is(e, os.ErrNotExist) && a.State == "deleting" {
			continue
		} else if e != nil {
			return ErrRecovery
		}
		if e = m.validate(v.ctx, a, noContent); e != nil {
			return safe(errors.Join(e, ErrRecovery))
		}
		union[r.key()] = r
	}
	next, e := orderedCatalog(union)
	if e != nil {
		return e
	}
	if e = m.physicalWith(v.ctx, rows, 0, next); e != nil {
		return e
	}
	return m.writeRetained(v.ctx, next)
}
