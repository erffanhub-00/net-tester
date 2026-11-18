package engine

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/erffanhub-00/net-tester/internal/config"
	"github.com/erffanhub-00/net-tester/internal/score"
)

// RunTargets tests every target in parallel (up to o.Workers at a
// time) and returns the results ranked by score, best first.
// Targets that fail score 0 and are listed last, unranked.
// Duplicate targets are tested once; the duplicate is reported
// with a warning on stderr.
func RunTargets(ctx context.Context, targets []config.Target, o Options) []*Result {
	seen := map[string]bool{}
	unique := make([]config.Target, 0, len(targets))
	for _, t := range targets {
		if seen[t.Key()] {
			fmt.Fprintf(os.Stderr, "warning: duplicate target %s — testing once\n", t.Key())
			continue
		}
		seen[t.Key()] = true
		unique = append(unique, t)
	}

	results := make([]*Result, len(unique))
	sem := make(chan struct{}, o.Workers)
	var wg sync.WaitGroup
	for i, t := range unique {
		wg.Add(1)
		go func(i int, t config.Target) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = RunOne(ctx, t, o)
		}(i, t)
	}
	wg.Wait()

	RankResults(results)
	ApplyICMPHints(results)
	return results
}

// RunOne tests a single target and scores it. The batch runner
// uses it for every target; the GUI uses it to report each target
// as it finishes.
func RunOne(ctx context.Context, t config.Target, o Options) *Result {
	start := time.Now()
	r := runOne(ctx, t, o)
	r.Duration = time.Since(start).Seconds() * 1000
	r.Score, r.Grade = score.Score(score.Input{
		Avg:        r.Avg,
		LossPct:    r.LossPct,
		Jitter:     r.Jitter,
		P95:        r.P95,
		Samples:    r.Received,
		HTTPBad:    r.HTTPStatus != 0 && (r.HTTPStatus < 200 || r.HTTPStatus >= 400),
		TLSInvalid: r.TLSValid != nil && !*r.TLSValid,
	})
	return r
}

// RankResults sorts results by score, best first, and assigns
// ranks. Failed targets score 0, are listed last and stay
// unranked (Rank 0). Ties break on lower average latency.
func RankResults(results []*Result) {
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].Avg < results[j].Avg
	})
	rank := 0
	for _, r := range results {
		if r.Failed() {
			r.Rank = 0
			continue
		}
		rank++
		r.Rank = rank
	}
}

// icmpHintNote is set on an ICMP result when the host answers on
// other methods but ping fails: ping is often filtered, e.g. by a
// VPN, while the host itself is up.
const icmpHintNote = "host answers on other methods — ICMP may be blocked (e.g. by a VPN)"

// ApplyICMPHints annotates ICMP results that failed (Timeout,
// Unreachable or Failed) when another result for the same host
// succeeded (OK or Partial). It runs after ranking; it only adds
// notes, never changes scores or statuses.
func ApplyICMPHints(results []*Result) {
	answers := map[string]bool{}
	for _, r := range results {
		if r.Status == StatusOK || r.Status == StatusPartial {
			answers[strings.ToLower(r.Target.Host)] = true
		}
	}
	for _, r := range results {
		if r.Target.Method != config.MethodICMP {
			continue
		}
		switch r.Status {
		case StatusTimeout, StatusUnreachable, StatusFailed:
		default:
			continue
		}
		if answers[strings.ToLower(r.Target.Host)] {
			r.Note = icmpHintNote
		}
	}
}

// runOne dispatches a target to the right test by method.
func runOne(ctx context.Context, t config.Target, o Options) *Result {
	switch t.Method {
	case config.MethodICMP:
		return ping(ctx, t, o.Packets, o.PingTimeout)
	case config.MethodTCP:
		return tcpTest(ctx, t, o.Packets, o.TCPTimeout)
	case config.MethodHTTP, config.MethodHTTPS:
		return httpTest(ctx, t, o)
	case config.MethodDNS:
		return dnsTest(ctx, t, o)
	default:
		return &Result{
			Target: t,
			Status: StatusFailed,
			Error:  fmt.Sprintf("unknown method %q", t.Method),
		}
	}
}

// AnyFailed reports whether any result failed.
func AnyFailed(results []*Result) bool {
	for _, r := range results {
		if r.Failed() {
			return true
		}
	}
	return false
}
