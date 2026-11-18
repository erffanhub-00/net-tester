package engine

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"time"

	"github.com/erffanhub-00/net-tester/internal/config"
)

// httpTest performs N HTTP(S) requests and measures per-request
// timings with httptrace: DNS (measured once, separately), connect
// time, TLS handshake time, time to first byte and total time.
// Redirects are followed (net/http default, up to 10).
//
// A response that arrives counts as a sample even when its status
// is an error code (404, 500, ...): the server answered, so the
// network path works. Only transport errors and timeouts count as
// loss. TLS validity is probed separately (see probeTLS): the
// per-sample client skips verification so a bad certificate does
// not hide the application measurement, while the probe reports
// the certificate problem honestly.
func httpTest(ctx context.Context, t config.Target, o Options) *Result {
	r := &Result{Target: t, Sent: o.Packets, HTTPDistribution: map[int]int{}}

	scheme := "http"
	if t.Method == config.MethodHTTPS {
		scheme = "https"
	}
	targetURL := scheme + "://" + net.JoinHostPort(t.Host, itoa(t.Port))

	// Resolve once first, with timing.
	ip, dnsMs, err := resolveHost(ctx, t.Host, o.HTTPTimeout)
	if err != nil {
		r.Status = StatusDNSError
		r.Error = err.Error()
		return r
	}
	r.ResolvedIP = ip
	r.DNSMs = dnsMs

	// TLS probe for HTTPS targets (once, not per sample).
	if t.Method == config.MethodHTTPS {
		valid, handshakeMs, err := probeTLS(ctx, t.Host, t.Port, o.HTTPTimeout)
		if err == nil {
			r.TLSValid = &valid
			r.TLSHandshakeMs = handshakeMs
		}
	}

	// dialMs holds the TCP connect time of the current request,
	// measured in DialContext below.
	var dialMs float64
	transport := &http.Transport{
		DisableKeepAlives: true,
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true}, // validity comes from probeTLS
		// Measure the TCP connect in the dialer itself: httptrace
		// ConnectStart/ConnectDone do not fire on every platform
		// (Windows CI measured connect=0 for localhost), while the
		// dial always runs, so this is the reliable measurement.
		DialContext: func(dialCtx context.Context, network, addr string) (net.Conn, error) {
			start := time.Now()
			conn, err := (&net.Dialer{Timeout: o.HTTPTimeout}).DialContext(dialCtx, network, addr)
			dialMs = time.Since(start).Seconds() * 1000
			return conn, err
		},
	}
	if o.Proxy != "" {
		if pu, err := url.Parse(o.Proxy); err == nil {
			transport.Proxy = http.ProxyURL(pu)
		}
	}
	client := &http.Client{Transport: transport, Timeout: o.HTTPTimeout}

	var timeouts int
	var connectTimes, ttfbTimes, totalTimes []float64
	for i := 0; i < o.Packets; i++ {
		if ctx.Err() != nil {
			break
		}
		var ttfbMs float64
		var firstByte time.Time
		dialMs = 0
		trace := &httptrace.ClientTrace{
			GotFirstResponseByte: func() { firstByte = time.Now() },
		}
		reqCtx := httptrace.WithClientTrace(ctx, trace)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, targetURL, nil)
		if err != nil {
			r.Error = err.Error()
			continue
		}
		start := time.Now()
		resp, err := client.Do(req)
		total := time.Since(start).Seconds() * 1000
		if err != nil {
			if isTimeout(err) {
				timeouts++
			}
			continue
		}
		// Drain the body so the connection can close cleanly.
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		ttfbMs = firstByte.Sub(start).Seconds() * 1000
		if ttfbMs <= 0 {
			// First byte was already in when Do returned (or the
			// trace hook did not fire); time-to-headers is the
			// honest fallback.
			ttfbMs = total
		}
		r.HTTPDistribution[resp.StatusCode]++
		r.Samples = append(r.Samples, total)
		connectTimes = append(connectTimes, dialMs)
		ttfbTimes = append(ttfbTimes, ttfbMs)
		totalTimes = append(totalTimes, total)
	}
	_ = totalTimes

	r.Received = len(r.Samples)
	if r.Sent > 0 {
		r.LossPct = float64(r.Sent-r.Received) / float64(r.Sent) * 100
	}
	stats := Summarise(r.Samples)
	r.Avg, r.Min, r.Max = stats.Avg, stats.Min, stats.Max
	r.Jitter, r.StdDev = stats.Jitter, stats.StdDev
	r.P50, r.P95, r.P99 = stats.P50, stats.P95, stats.P99
	r.ConnectMs = avgOf(connectTimes)
	r.TTFBMs = avgOf(ttfbTimes)
	r.TotalMs = r.Avg
	r.HTTPStatus = dominantStatus(r.HTTPDistribution)

	switch {
	case r.Received == 0 && timeouts == r.Sent:
		r.Status = StatusTimeout
		r.Error = "request timed out"
	case r.Received == 0:
		r.Status = StatusUnreachable
		r.Error = "no response"
	default:
		r.Status = LossStatus(r.LossPct)
	}
	return r
}

func avgOf(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sum := 0.0
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}

// dominantStatus returns the most common status code.
func dominantStatus(dist map[int]int) int {
	best, bestCount := 0, 0
	for code, n := range dist {
		if n > bestCount {
			best, bestCount = code, n
		}
	}
	return best
}

// probeTLS dials host:port once with full certificate verification
// and returns whether the certificate is valid plus the handshake
// time in milliseconds. A certificate error returns valid=false
// with a nil error (the handshake itself completed far enough to
// judge the certificate); network errors return an error.
func probeTLS(ctx context.Context, host string, port int, timeout time.Duration) (bool, float64, error) {
	d := net.Dialer{Timeout: timeout}
	start := time.Now()
	conn, err := tls.DialWithDialer(&d, "tcp", net.JoinHostPort(host, itoa(port)), &tls.Config{
		ServerName: host,
	})
	ms := time.Since(start).Seconds() * 1000
	if err != nil {
		var certErr *tls.CertificateVerificationError
		if errors.As(err, &certErr) {
			return false, ms, nil
		}
		// Hostname mismatch and other verification failures also
		// surface as certificate errors in Go's TLS stack.
		if strings.Contains(err.Error(), "certificate") {
			return false, ms, nil
		}
		return false, ms, err
	}
	conn.Close()
	return true, ms, nil
}
