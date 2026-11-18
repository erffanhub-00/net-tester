package engine

import (
	"bufio"
	"context"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// ParsePingReplyLine extracts the latency from one ping reply line
// in either dialect:
//
//	Windows: Reply from 8.8.8.8: bytes=32 time=12ms TTL=117
//	Windows: Reply from 8.8.8.8: bytes=32 time<1ms TTL=117
//	Linux:   64 bytes from 8.8.8.8: icmp_seq=1 ttl=117 time=11.2 ms
//
// It returns ok=false for any line that is not a reply (timeouts,
// unreachable notices, statistics, headers). A "time<1ms" reply is
// reported as 0.5 ms, the same convention parsePingOutput uses.
func ParsePingReplyLine(line string) (ms float64, ok bool) {
	if strings.Contains(line, "time<1ms") {
		return 0.5, true
	}
	if m := replyTime.FindStringSubmatch(line); m != nil {
		v, err := strconv.ParseFloat(m[1], 64)
		if err == nil {
			return v, true
		}
	}
	return 0, false
}

// LivePingStats accumulates the results of a continuous ping run.
type LivePingStats struct {
	Sent     int
	Received int
	Samples  []float64 // reply latencies in ms, in arrival order
}

// AddReply records a received reply.
func (s *LivePingStats) AddReply(ms float64) {
	s.Sent++
	s.Received++
	s.Samples = append(s.Samples, ms)
}

// AddMiss records a sent request with no reply (timeout or error
// line from ping).
func (s *LivePingStats) AddMiss() {
	s.Sent++
}

// LossPct is the percentage of sent requests with no reply.
func (s *LivePingStats) LossPct() float64 {
	if s.Sent == 0 {
		return 0
	}
	return float64(s.Sent-s.Received) / float64(s.Sent) * 100
}

// Summary summarises the received latencies (avg/min/max, jitter,
// percentiles) with the same definition as the batch tests.
func (s *LivePingStats) Summary() Stats {
	return Summarise(s.Samples)
}

// LivePingArgs returns the arguments for a continuous ping: on
// Windows `ping -t host` runs until stopped; on Unix a bare
// `ping host` does the same.
func LivePingArgs(host string) []string {
	if runtime.GOOS == "windows" {
		return []string{"-t", host}
	}
	return []string{host}
}

// StreamPing runs the system ping continuously and calls onLine for
// every line of output, until ctx is cancelled or ping exits. The
// caller parses lines (see ParsePingReplyLine) and keeps the UI
// responsive: StreamPing blocks, so run it on a goroutine.
func StreamPing(ctx context.Context, host string, onLine func(string)) error {
	cmd := exec.CommandContext(ctx, "ping", LivePingArgs(host)...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return err
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		onLine(scanner.Text())
	}
	_ = cmd.Wait()
	return scanner.Err()
}
