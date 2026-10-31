package agent

import "github.com/hidxt/miskoai/internal/netx"

// Optional diagnostic destinations are local to one Handle invocation. Existing
// internal callers need no destination; no shared Agent state is modified.
func recordModelFailure(destinations []*string, err error) {
	if len(destinations) != 0 && destinations[0] != nil {
		*destinations[0] = netx.FailureCode(err)
	}
}
