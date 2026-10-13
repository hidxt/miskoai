package library

import "context"

func (m *Manager) begin(parent context.Context) (context.Context, func(), error) {
	if m == nil || parent == nil {
		return nil, nil, ErrInvalid
	}
	select {
	case <-m.stop:
		return nil, nil, ErrClosed
	default:
	}
	if e := parent.Err(); e != nil {
		return nil, nil, e
	}
	select {
	case m.token <- struct{}{}:
	case <-m.stop:
		return nil, nil, ErrClosed
	case <-parent.Done():
		return nil, nil, parent.Err()
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		<-m.token
		return nil, nil, ErrClosed
	}
	ctx, cancel := context.WithCancel(parent)
	m.active = cancel
	m.mu.Unlock()
	return ctx, func() { cancel(); m.mu.Lock(); m.active = nil; m.mu.Unlock(); <-m.token }, nil
}

// Close stops admission, cancels the current operation, and joins its actual
// reads, consumer, artifact Close, persistent Close, and cleanup. No I/O is
// detached and no wall-clock deadline is promised for an uncooperative caller.
func (m *Manager) Close() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	if m.closed {
		done := m.closeDone
		m.mu.Unlock()
		<-done
		return nil
	}
	m.closed = true
	close(m.stop)
	if m.active != nil {
		m.active()
	}
	m.mu.Unlock()
	m.token <- struct{}{}
	close(m.closeDone)
	<-m.token
	return nil
}
