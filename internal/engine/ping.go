package engine

import (
	"context"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/erffanhub-00/net-tester/internal/config"
)

// ping runs the system ping binary for one target and parses the
// output. Go has no ICMP in the standard library and x/net/icmp
// needs admin/root privileges, so shelling out to ping is the
// pragmatic choice — the same approach Network-Ping-Pro used.
// It works unprivileged on Windows, Linux and macOS.
//
// Both Windows and Unix ping output are parsed, so the same code
// runs on every platform and the parser is covered by golden tests.
func ping(ctx context.Context, t config.Target, packets int, timeout time.Duration) *Result {
	r := &Result{Target: t, Sent: packets}

	// Overall deadline: every reply may take up to `timeout`.
	deadline := time.Duration(packets)*(timeout+time.Second) + 5*time.Second
	ctx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()

	args, err := pingArgs(t.Host, packets, timeout)
	if err != nil {
		r.Status = StatusFailed
		r.Error = err.Error()
		return r
	}
	cmd := exec.CommandContext(ctx, "ping", args...)
	hidePingWindow(cmd)
	out, err := cmd.CombinedOutput()
	text := string(out)

	samples, sent, received, dnsErr := parsePingOutput(text, packets)
	r.Samples = samples
	if sent > 0 {
		r.Sent = sent
	}
	r.Received = received
	if r.Sent > 0 {
		r.LossPct = float64(r.Sent-r.Received) / float64(r.Sent) * 100
	}

	// Resolve the IP for display (cheap: ping already resolved it,
	// but the parsed IP is more reliable across platforms).
	if ip := parsePingIP(text); ip != "" {
		r.ResolvedIP = ip
	}

	stats := Summarise(samples)
	r.Avg, r.Min, r.Max = stats.Avg, stats.Min, stats.Max
	r.Jitter, r.StdDev = stats.Jitter, stats.StdDev
	r.P50, r.P95, r.P99 = stats.P50, stats.P95, stats.P99

	switch {
	case dnsErr:
		r.Status = StatusDNSError
		r.Error = "host could not be resolved"
	case received == 0:
		if strings.Contains(text, "unreachable") || strings.Contains(text, "Unreachable") {
			r.Status = StatusUnreachable
		} else if err != nil && ctx.Err() == context.DeadlineExceeded {
			r.Status = StatusTimeout
		} else {
			r.Status = StatusTimeout
		}
		if r.Error == "" && err != nil {
			r.Error = err.Error()
		}
	default:
		r.Status = LossStatus(r.LossPct)
	}
	return r
}

// pingArgs builds the platform-specific ping command line.
func pingArgs(host string, packets int, timeout time.Duration) ([]string, error) {
	if packets < 1 {
		return nil, errInvalidPackets
	}
	switch runtime.GOOS {
	case "windows":
		// -n count, -w per-reply timeout in milliseconds.
		ms := int(timeout.Milliseconds())
		if ms < 1 {
			ms = 1
		}
		return []string{"-n", strconv.Itoa(packets), "-w", strconv.Itoa(ms), host}, nil
	case "darwin":
		// -c count, -t overall timeout in seconds.
		secs := int(timeout.Seconds())*packets + 2
		if secs < 1 {
			secs = 1
		}
		return []string{"-c", strconv.Itoa(packets), "-t", strconv.Itoa(secs), host}, nil
	default:
		// Linux and the rest: -c count, -W per-reply timeout in seconds.
		secs := int(timeout.Seconds())
		if secs < 1 {
			secs = 1
		}
		return []string{"-c", strconv.Itoa(packets), "-W", strconv.Itoa(secs), host}, nil
	}
}

var errInvalidPackets = &simpleError{"packets must be at least 1"}

type simpleError struct{ msg string }

func (e *simpleError) Error() string { return e.msg }

// replyTime matches one reply line in either ping dialect:
//
//	Windows: Reply from 8.8.8.8: bytes=32 time=12ms TTL=117
//	Windows: Reply from 8.8.8.8: bytes=32 time<1ms TTL=117
//	Linux:   64 bytes from 8.8.8.8: icmp_seq=1 ttl=117 time=11.2 ms
//	macOS:   64 bytes from 8.8.8.8: icmp_seq=0 ttl=117 time=11.234 ms
var replyTime = regexp.MustCompile(`time\s*[=<]\s*(\d+(?:\.\d+)?)\s*ms`)

// windowsStats matches "Packets: Sent = 4, Received = 4, Lost = 0 (0% loss),".
var windowsStats = regexp.MustCompile(`Sent\s*=\s*(\d+),\s*Received\s*=\s*(\d+),\s*Lost\s*=\s*(\d+)`)

// unixStats matches "4 packets transmitted, 4 received, 0% packet loss"
// and the macOS variant "4 packets transmitted, 4 packets received, 0.0% packet loss".
var unixStats = regexp.MustCompile(`(\d+)\s+packets transmitted,\s*(\d+)\s+(?:packets\s+)?received`)

// pingIP matches the resolved address in the header line:
// "Pinging 8.8.8.8 ..." (Windows) or "PING host (8.8.8.8) ..." (Unix).
var pingIP = regexp.MustCompile(`PING[^\(]*\(([^\)]+)\)`)

var dnsFailureMarkers = []string{
	"could not find host",
	"unknown host",
	"Name or service not known",
	"Temporary failure in name resolution",
	"cannot resolve",
	"Unknown host",
}

// parsePingOutput extracts reply times and sent/received counts from
// ping output in either dialect. dnsErr is true when the output
// shows the host could not be resolved at all.
func parsePingOutput(text string, wantPackets int) (samples []float64, sent, received int, dnsErr bool) {
	sent = wantPackets
	for _, line := range strings.Split(text, "\n") {
		lower := strings.ToLower(line)
		for _, m := range dnsFailureMarkers {
			if strings.Contains(lower, strings.ToLower(m)) {
				dnsErr = true
			}
		}
		if strings.Contains(line, "time<1ms") {
			// Windows reports sub-millisecond replies as "<1ms";
			// record half a millisecond rather than 1.
			samples = append(samples, 0.5)
			continue
		}
		if m := replyTime.FindStringSubmatch(line); m != nil {
			ms, err := strconv.ParseFloat(m[1], 64)
			if err == nil {
				samples = append(samples, ms)
			}
			continue
		}
		if m := windowsStats.FindStringSubmatch(line); m != nil {
			if s, err := strconv.Atoi(m[1]); err == nil {
				sent = s
			}
			if rc, err := strconv.Atoi(m[2]); err == nil {
				received = rc
			}
		}
		if m := unixStats.FindStringSubmatch(line); m != nil {
			if s, err := strconv.Atoi(m[1]); err == nil {
				sent = s
			}
			if rc, err := strconv.Atoi(m[2]); err == nil {
				received = rc
			}
		}
	}
	// Fall back to what we actually saw if no stats line matched.
	if received == 0 {
		received = len(samples)
	}
	if received > sent {
		received = sent
	}
	return samples, sent, received, dnsErr
}

// parsePingIP extracts the resolved IP from the ping header line.
func parsePingIP(text string) string {
	if m := pingIP.FindStringSubmatch(text); m != nil {
		return m[1]
	}
	// Windows header has no parentheses: "Pinging 8.8.8.8 with 32 bytes of data:"
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "Pinging ") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				return strings.TrimSuffix(fields[1], ":")
			}
		}
	}
	return ""
}
