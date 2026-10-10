package netx

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"time"
)

// NewPublicTransport pins each connection to a validated public DNS address.
// Callers still own URL authorization, HTTP deadlines, redirect/retry policy,
// credentials, operation admission and body limits. Configure before use.
func NewPublicTransport(timeout time.Duration) (*http.Transport, error) {
	if timeout <= 0 || timeout > 2*time.Minute {
		return nil, errors.New("invalid transport bounds")
	}
	return &http.Transport{
		DialContext:            safeDial,
		TLSClientConfig:        &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:    10 * time.Second,
		ResponseHeaderTimeout:  timeout,
		MaxIdleConns:           4,
		MaxIdleConnsPerHost:    2,
		MaxConnsPerHost:        2,
		IdleConnTimeout:        60 * time.Second,
		DisableCompression:     true,
		MaxResponseHeaderBytes: 32 << 10,
	}, nil
}

func safeDial(ctx context.Context, network, address string) (net.Conn, error) {
	d := net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	return dialPublicAddress(ctx, network, address, net.DefaultResolver.LookupNetIP, d.DialContext)
}

// Both real operations and synthetic per-call fixtures use this same guard.
// Validate the complete answer set before dialing any address; never pass the
// original hostname to the dialer after the lookup.
func dialPublicAddress(ctx context.Context, network, address string,
	lookup func(context.Context, string, string) ([]netip.Addr, error),
	dial func(context.Context, string, string) (net.Conn, error)) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, errors.New("invalid remote address")
	}
	addresses, err := lookup(ctx, "ip", host)
	if err != nil {
		return nil, errors.New("remote DNS failure")
	}
	if len(addresses) == 0 {
		return nil, errors.New("remote DNS empty")
	}
	for _, ip := range addresses {
		if !PublicIP(ip) {
			return nil, errors.New("private remote address rejected")
		}
	}
	for _, ip := range addresses {
		conn, err := dial(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
	}
	return nil, errors.New("remote connection failure")
}
