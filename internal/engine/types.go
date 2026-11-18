// Package engine runs the network tests: ICMP ping (via the system
// ping binary), TCP connect, HTTP/HTTPS and real DNS resolution
// checks. All measurements are done per target over N samples.
package engine

import (
	"math"
	"sort"
	"time"

	"github.com/erffanhub-00/net-tester/internal/config"
)

// Target statuses. A target is Failed when it produced no usable
// sample at all; otherwise the status describes the outcome.
const (
	StatusOK          = "OK"
	StatusPartial     = "Partial"   // loss below 20%
	StatusHighLoss    = "High Loss" // loss 20% or more
	StatusRefused     = "Refused"   // TCP connect refused on every sample
	StatusTimeout     = "Timeout"   // every sample timed out
	StatusDNSError    = "DNS Error" // host could not be resolved
	StatusUnreachable = "Unreachable"
	StatusFailed      = "Failed"
)

// Options controls one test run.
type Options struct {
	Packets     int           // samples per target, 1-200
	Workers     int           // parallel targets, 1-50
	PingTimeout time.Duration // per-reply timeout for ping
	TCPTimeout  time.Duration // per-attempt TCP connect timeout
	HTTPTimeout time.Duration // per-request HTTP timeout
	DNSTimeout  time.Duration // per-lookup DNS timeout
	Proxy       string        // HTTP proxy URL (HTTP only), empty = none
	DNSTestHost string        // hostname resolved by DNS-method targets
}

// DefaultOptions returns the standard run options.
func DefaultOptions() Options {
	return Options{
		Packets:     10,
		Workers:     10,
		PingTimeout: 2 * time.Second,
		TCPTimeout:  3 * time.Second,
		HTTPTimeout: 5 * time.Second,
		DNSTimeout:  3 * time.Second,
		DNSTestHost: "example.com",
	}
}

// Result is the measured outcome for one target.
type Result struct {
	Target  config.Target `json:"target"`
	Status  string        `json:"status"`
	Samples []float64     `json:"samples_ms,omitempty"`

	Sent     int     `json:"sent"`
	Received int     `json:"received"`
	LossPct  float64 `json:"loss_pct"`

	Avg    float64 `json:"avg_ms"`
	Min    float64 `json:"min_ms"`
	Max    float64 `json:"max_ms"`
	Jitter float64 `json:"jitter_ms"`
	StdDev float64 `json:"stddev_ms"`
	P50    float64 `json:"p50_ms"`
	P95    float64 `json:"p95_ms"`
	P99    float64 `json:"p99_ms"`

	ResolvedIP string  `json:"resolved_ip,omitempty"`
	DNSMs      float64 `json:"dns_ms,omitempty"`
	ConnectMs  float64 `json:"connect_ms,omitempty"`
	TTFBMs     float64 `json:"ttfb_ms,omitempty"`
	TotalMs    float64 `json:"total_ms,omitempty"`

	HTTPStatus       int         `json:"http_status,omitempty"`
	HTTPDistribution map[int]int `json:"http_distribution,omitempty"`
	TLSValid         *bool       `json:"tls_valid,omitempty"`
	TLSHandshakeMs   float64     `json:"tls_handshake_ms,omitempty"`

	Score    float64 `json:"score"`
	Grade    string  `json:"grade"`
	Rank     int     `json:"rank"` // 0 = unranked (failed)
	Duration float64 `json:"duration_ms"`
	Error    string  `json:"error,omitempty"`
}

// Failed reports whether the target produced no usable sample.
func (r *Result) Failed() bool {
	return r.Received == 0
}

// DisplayStatus returns the status string shown to the user. For an
// HTTP target whose requests all returned the same non-2xx/3xx
// status, it shows the code (e.g. "HTTP 404").
func (r *Result) DisplayStatus() string {
	if r.Target.Method == config.MethodHTTP || r.Target.Method == config.MethodHTTPS {
		if r.HTTPStatus != 0 && (r.HTTPStatus < 200 || r.HTTPStatus >= 400) && r.Received > 0 {
			if len(r.HTTPDistribution) == 1 {
				return "HTTP " + itoa(r.HTTPStatus)
			}
		}
	}
	return r.Status
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// Stats summarises a sample list. Times are milliseconds.
type Stats struct {
	Avg, Min, Max, Jitter, StdDev, P50, P95, P99 float64
}

// Summarise computes avg/min/max, jitter, stddev and percentiles
// from a list of per-sample times.
//
// Jitter is the mean absolute difference between consecutive
// samples. This is deliberately NOT RFC 3550 jitter: it matches
// the definition both predecessor tools used, and it is easy to
// explain. Percentiles use linear interpolation.
func Summarise(samples []float64) Stats {
	var s Stats
	if len(samples) == 0 {
		return s
	}
	sorted := append([]float64(nil), samples...)
	sort.Float64s(sorted)
	s.Min = sorted[0]
	s.Max = sorted[len(sorted)-1]
	sum := 0.0
	for _, v := range samples {
		sum += v
	}
	s.Avg = sum / float64(len(samples))
	if len(samples) > 1 {
		diffSum := 0.0
		for i := 1; i < len(samples); i++ {
			diffSum += math.Abs(samples[i] - samples[i-1])
		}
		s.Jitter = diffSum / float64(len(samples)-1)
		var sqSum float64
		for _, v := range samples {
			d := v - s.Avg
			sqSum += d * d
		}
		s.StdDev = math.Sqrt(sqSum / float64(len(samples)))
	}
	s.P50 = percentile(sorted, 50)
	s.P95 = percentile(sorted, 95)
	s.P99 = percentile(sorted, 99)
	return s
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	rank := p / 100 * float64(len(sorted)-1)
	lo := int(math.Floor(rank))
	hi := int(math.Ceil(rank))
	if lo == hi {
		return sorted[lo]
	}
	frac := rank - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}

// LossStatus derives OK/Partial/HighLoss from a loss percentage.
// Callers set Refused/Timeout/DNS Error/Unreachable themselves.
func LossStatus(lossPct float64) string {
	switch {
	case lossPct <= 0:
		return StatusOK
	case lossPct < 20:
		return StatusPartial
	default:
		return StatusHighLoss
	}
}
