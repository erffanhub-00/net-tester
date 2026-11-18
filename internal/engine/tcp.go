package engine

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/erffanhub-00/net-tester/internal/config"
)

// tcpTest dials host:port N times and measures connect time.
// A refused connection means the host is reachable but the port
// is closed — that is reported as Refused, not as a generic failure.
func tcpTest(ctx context.Context, t config.Target, packets int, timeout time.Duration) *Result {
	r := &Result{Target: t, Sent: packets}

	// Resolve once first, with timing. A resolution failure is a
	// DNS error, not a TCP failure.
	ip, dnsMs, err := resolveHost(ctx, t.Host, timeout)
	if err != nil {
		r.Status = StatusDNSError
		r.Error = err.Error()
		return r
	}
	r.ResolvedIP = ip
	r.DNSMs = dnsMs

	addr := net.JoinHostPort(t.Host, itoa(t.Port))
	var refused, timeouts int
	for i := 0; i < packets; i++ {
		if ctx.Err() != nil {
			break
		}
		start := time.Now()
		d := net.Dialer{Timeout: timeout}
		conn, err := d.DialContext(ctx, "tcp", addr)
		elapsed := time.Since(start).Seconds() * 1000
		if err != nil {
			if isRefused(err) {
				refused++
			} else if isTimeout(err) {
				timeouts++
			}
			continue
		}
		conn.Close()
		r.Samples = append(r.Samples, elapsed)
	}
	r.Received = len(r.Samples)
	if r.Sent > 0 {
		r.LossPct = float64(r.Sent-r.Received) / float64(r.Sent) * 100
	}
	stats := Summarise(r.Samples)
	r.Avg, r.Min, r.Max = stats.Avg, stats.Min, stats.Max
	r.Jitter, r.StdDev = stats.Jitter, stats.StdDev
	r.P50, r.P95, r.P99 = stats.P50, stats.P95, stats.P99
	// For TCP the "connect time" is the latency itself.
	r.ConnectMs = r.Avg

	switch {
	case r.Received == 0 && refused == packets:
		r.Status = StatusRefused
		r.Error = "connection refused"
	case r.Received == 0 && timeouts == packets:
		r.Status = StatusTimeout
		r.Error = "connection timed out"
	case r.Received == 0:
		r.Status = StatusUnreachable
		r.Error = "host unreachable"
	default:
		r.Status = LossStatus(r.LossPct)
	}
	return r
}

// resolveHost resolves a hostname once and returns the first IP
// plus the resolution time in milliseconds.
func resolveHost(ctx context.Context, host string, timeout time.Duration) (string, float64, error) {
	if ip := net.ParseIP(host); ip != nil {
		return host, 0, nil
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	addrs, err := net.DefaultResolver.LookupHost(ctx, host)
	ms := time.Since(start).Seconds() * 1000
	if err != nil {
		return "", ms, err
	}
	if len(addrs) == 0 {
		return "", ms, errors.New("no addresses found for " + host)
	}
	return addrs[0], ms, nil
}

func isRefused(err error) bool {
	return strings.Contains(err.Error(), "connection refused")
}

func isTimeout(err error) bool {
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	return strings.Contains(err.Error(), "i/o timeout") ||
		strings.Contains(err.Error(), "timeout")
}
