package engine

import (
	"context"
	"net"
	"time"

	"github.com/erffanhub-00/net-tester/internal/config"
)

// dnsTest checks a DNS resolver by asking it to resolve a hostname.
// This is a REAL DNS check with timing — unlike server-tester's
// old "DNS" category, which only TCP-connected to port 53 and
// never sent a query. The target host is the resolver (usually an
// IP like 8.8.8.8); o.DNSTestHost is the name it must resolve.
func dnsTest(ctx context.Context, t config.Target, o Options) *Result {
	r := &Result{Target: t, Sent: o.Packets}

	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: o.DNSTimeout}
			// The target's port is the resolver port; 0 means the
			// standard DNS port 53.
			port := t.Port
			if port == 0 {
				port = 53
			}
			return d.DialContext(ctx, "udp", net.JoinHostPort(t.Host, itoa(port)))
		},
	}

	var timeouts int
	for i := 0; i < o.Packets; i++ {
		if ctx.Err() != nil {
			break
		}
		lctx, cancel := context.WithTimeout(ctx, o.DNSTimeout)
		start := time.Now()
		addrs, err := resolver.LookupHost(lctx, o.DNSTestHost)
		elapsed := time.Since(start).Seconds() * 1000
		cancel()
		if err != nil {
			if isTimeout(err) {
				timeouts++
			}
			continue
		}
		if len(addrs) > 0 && r.ResolvedIP == "" {
			r.ResolvedIP = addrs[0]
		}
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
	r.DNSMs = r.Avg

	switch {
	case r.Received == 0 && timeouts == r.Sent:
		r.Status = StatusTimeout
		r.Error = "DNS query timed out"
	case r.Received == 0:
		r.Status = StatusDNSError
		r.Error = "resolver did not answer"
	default:
		r.Status = LossStatus(r.LossPct)
	}
	return r
}
