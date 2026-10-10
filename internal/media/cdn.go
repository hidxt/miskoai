package media

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/hidxt/miskoai/internal/netx"
)

var (
	ErrEndpoint  = errors.New("media endpoint")
	ErrHTTP      = errors.New("media HTTP")
	ErrTransport = errors.New("media transport")
	ErrClosed    = errors.New("media closed")
)

// CDN owns one actual transfer lifetime. It sends no API credentials, uses no
// replay source and never starts a goroutine to abandon a blocked Reader.
// Configure the private client before use; it is not an application seam.
type CDN struct {
	client       *http.Client
	slot         chan struct{}
	stop, done   chan struct{}
	mu           sync.Mutex
	closed       bool
	activeCancel context.CancelFunc
	work         sync.WaitGroup
}

// Close joins any active borrowed read without taking Artifact ownership.
// RoundTripper may close request bodies asynchronously after returning, so
// Upload also closes this wrapper itself before releasing admission.
type cdnUploadBody struct {
	mu       sync.Mutex
	reader   io.Reader
	closed   bool
	consumed int64
	readErr  error
}

func (b *cdnUploadBody) Read(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return 0, ErrIO
	}
	n, e := b.reader.Read(p)
	if n < 0 || n > len(p) {
		b.readErr = ErrIO
		return 0, ErrIO
	}
	b.consumed += int64(n)
	if e != nil && e != io.EOF && b.readErr == nil {
		b.readErr = ErrIO
	}
	return n, e
}
func (b *cdnUploadBody) Close() error { b.mu.Lock(); defer b.mu.Unlock(); b.closed = true; return nil }

func (b *cdnUploadBody) complete(expected int64) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	if b.readErr != nil || b.consumed != expected {
		return ErrIO
	}
	return nil
}

func NewCDN() (*CDN, error) {
	transport, e := netx.NewPublicTransport(30 * time.Second)
	if e != nil {
		return nil, ErrTransport
	}
	// net/http can replay a GET after a failed reused connection. A fresh
	// connection per transfer prevents that implicit retry path as well. Explicit
	// HTTP1 also excludes the HTTP2 NoCachedConn retry path that precedes the
	// fresh-connection check in the installed standard library.
	transport.DisableKeepAlives = true
	transport.Protocols = new(http.Protocols)
	transport.Protocols.SetHTTP1(true)
	return &CDN{client: &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, slot: make(chan struct{}, 1), stop: make(chan struct{}), done: make(chan struct{})}, nil
}

// begin includes admission wait in the same fixed30s deadline as actual work.
func (c *CDN) begin(parent context.Context) (context.Context, func(), error) {
	if parent == nil || c == nil {
		return nil, nil, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	if e := ctx.Err(); e != nil {
		cancel()
		return nil, nil, e
	}
	select {
	case <-c.stop:
		cancel()
		return nil, nil, ErrClosed
	case <-ctx.Done():
		cancel()
		return nil, nil, ctx.Err()
	case c.slot <- struct{}{}:
	}
	c.mu.Lock()
	if c.closed || ctx.Err() != nil {
		closed := c.closed
		c.mu.Unlock()
		<-c.slot
		cancel()
		if closed {
			return nil, nil, ErrClosed
		}
		return nil, nil, ctx.Err()
	}
	c.activeCancel = cancel
	c.work.Add(1)
	c.mu.Unlock()
	finish := func() {
		cancel()
		c.mu.Lock()
		c.activeCancel = nil
		c.mu.Unlock()
		<-c.slot
		c.work.Done()
	}
	return ctx, finish, nil
}

func safeCDNError(ctx context.Context, e error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(e, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(e, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	switch e {
	case ErrInvalid, ErrLimit, ErrPrivate, ErrIO, ErrEndpoint, ErrHTTP, ErrTransport, ErrClosed:
		return e
	}
	return ErrTransport
}

func identityEncoding(h http.Header) bool {
	v := h.Values("Content-Encoding")
	return len(v) == 0 || (len(v) == 1 && (v[0] == "" || strings.EqualFold(v[0], "identity")))
}

// Download returns a caller-owned completed private plaintext artifact only
// after decryption and response Close both succeed.
func (c *CDN) Download(parent context.Context, dataDir, encryptedQuery, fullURL string, key [16]byte) (artifact *Artifact, err error) {
	ctx, finish, e := c.begin(parent)
	if e != nil {
		return nil, e
	}
	defer finish()
	raw, e := cdnURL("download", encryptedQuery, fullURL, "")
	if e != nil {
		return nil, e
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if e != nil {
		return nil, ErrInvalid
	}
	resp, e := c.client.Do(req)
	if e != nil {
		return nil, safeCDNError(ctx, e)
	}
	if resp.Body == nil {
		return nil, ErrTransport
	}
	// Close remains inside admission, and failure removes a just-completed file.
	defer func() {
		closeErr := resp.Body.Close()
		if ctx.Err() != nil {
			err = ctx.Err()
		} else if closeErr != nil && err == nil {
			err = ErrIO
		}
		if err != nil && artifact != nil {
			if artifact.Close() != nil && ctx.Err() == nil {
				err = ErrIO
			}
			artifact = nil
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return nil, ErrHTTP
	}
	if !identityEncoding(resp.Header) {
		return nil, ErrInvalid
	}
	if resp.ContentLength > maxCiphertext {
		return nil, ErrLimit
	}
	artifact, e = Decrypt(ctx, dataDir, resp.Body, key)
	if e != nil {
		return nil, safeCDNError(ctx, e)
	}
	return artifact, nil
}

// Upload borrows a completed ciphertext Artifact; it never closes or removes
// the caller's artifact. The source has no GetBody and cannot be replayed.
// Protocol route/header details follow Tencent/openclaw-weixin at commit
// 24de5c9eb0dd5e595d7e2d090ed8a3f82870d42c (MIT; internal/notices/notices.txt).
func (c *CDN) Upload(parent context.Context, ciphertext *Artifact, uploadParam, fullURL, filekey string) (parameter string, err error) {
	ctx, finish, e := c.begin(parent)
	if e != nil {
		return "", e
	}
	defer finish()
	raw, e := cdnURL("upload", uploadParam, fullURL, filekey)
	if e != nil {
		return "", e
	}
	if ciphertext == nil || ciphertext.Size() <= 0 || ciphertext.Size()%16 != 0 {
		return "", ErrInvalid
	}
	if ciphertext.Size() > maxCiphertext {
		return "", ErrLimit
	}
	borrow := &cdnUploadBody{reader: io.NewSectionReader(ciphertext, 0, ciphertext.Size())}
	defer func() {
		borrowErr := borrow.complete(ciphertext.Size())
		// A cooperative borrowed read can finish after the response-close defer.
		// Cancellation during this final actual join still refuses success.
		if ctx.Err() != nil {
			err = ctx.Err()
			parameter = ""
		} else if borrowErr != nil && err == nil {
			err = borrowErr
			parameter = ""
		}
	}()
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, raw, borrow)
	if e != nil {
		return "", ErrInvalid
	}
	req.ContentLength = ciphertext.Size()
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, e := c.client.Do(req)
	if e != nil {
		return "", safeCDNError(ctx, e)
	}
	if resp.Body == nil {
		return "", ErrTransport
	}
	defer func() {
		closeErr := resp.Body.Close()
		if ctx.Err() != nil {
			err = ctx.Err()
		} else if closeErr != nil && err == nil {
			err = ErrIO
		}
		if err != nil {
			parameter = ""
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return "", ErrHTTP
	}
	if !identityEncoding(resp.Header) {
		return "", ErrInvalid
	}
	values := resp.Header.Values("x-encrypted-param")
	if len(values) != 1 || values[0] == "" || !cleanMetadata(values[0], 4096, false) {
		return "", ErrInvalid
	}
	if resp.ContentLength > 64<<10 {
		return "", ErrLimit
	}
	if e = drainCDN(ctx, resp.Body); e != nil {
		return "", e
	}
	return values[0], nil
}

func drainCDN(ctx context.Context, r io.Reader) error {
	var buffer [4096]byte
	total, empty := 0, 0
	for {
		if e := ctx.Err(); e != nil {
			return e
		}
		size := len(buffer)
		if remain := (64 << 10) - total + 1; size > remain {
			size = remain
		}
		n, e := r.Read(buffer[:size])
		if err := ctx.Err(); err != nil {
			return err
		}
		if n < 0 || n > size {
			return ErrIO
		}
		total += n
		if total > 64<<10 {
			return ErrLimit
		}
		if e != nil && e != io.EOF {
			return ErrIO
		}
		if e == io.EOF {
			return nil
		}
		if n == 0 {
			empty++
			if empty >= 100 {
				return ErrIO
			}
		} else {
			empty = 0
		}
	}
}

// Close refuses new work, cancels admitted work and joins its actual body and
// filesystem lifetime. A noncooperative Reader may hold Close until it returns.
// Calling Close from inside an admitted transfer callback is unsupported.
func (c *CDN) Close() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		<-c.done
		return nil
	}
	c.closed = true
	close(c.stop)
	if c.activeCancel != nil {
		c.activeCancel()
	}
	c.mu.Unlock()
	c.work.Wait()
	c.client.CloseIdleConnections()
	close(c.done)
	return nil
}
