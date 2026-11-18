package score

import "testing"

func TestScorePerfect(t *testing.T) {
	s, g := Score(Input{Avg: 0, LossPct: 0, Jitter: 0, P95: 0, Samples: 10})
	if s != 100 || g != GradeExcellent {
		t.Fatalf("perfect target: score=%v grade=%q, want 100 Excellent", s, g)
	}
	// A near-perfect target (5ms avg, 1ms jitter) scores just under 100.
	s, _ = Score(Input{Avg: 5, LossPct: 0, Jitter: 1, P95: 10, Samples: 10})
	if s != 99.1 {
		t.Fatalf("near-perfect target: score=%v, want 99.1", s)
	}
}

func TestScoreFailed(t *testing.T) {
	s, g := Score(Input{Samples: 0})
	if s != 0 || g != GradeFailed {
		t.Fatalf("failed target: score=%v grade=%q, want 0 Failed", s, g)
	}
}

func TestScoreLossHurts(t *testing.T) {
	full, _ := Score(Input{Avg: 10, LossPct: 0, Jitter: 1, P95: 20, Samples: 10})
	half, _ := Score(Input{Avg: 10, LossPct: 50, Jitter: 1, P95: 20, Samples: 5})
	if half >= full {
		t.Fatalf("50%% loss should score lower: full=%v half=%v", full, half)
	}
}

func TestScoreP95Penalty(t *testing.T) {
	fast, _ := Score(Input{Avg: 10, LossPct: 0, Jitter: 1, P95: 100, Samples: 10})
	slow, _ := Score(Input{Avg: 10, LossPct: 0, Jitter: 1, P95: 600, Samples: 10})
	if slow >= fast {
		t.Fatalf("slow p95 should score lower: fast=%v slow=%v", fast, slow)
	}
}

func TestScoreHTTPBadPenalty(t *testing.T) {
	ok, _ := Score(Input{Avg: 10, LossPct: 0, Jitter: 1, P95: 20, Samples: 10})
	bad, _ := Score(Input{Avg: 10, LossPct: 0, Jitter: 1, P95: 20, Samples: 10, HTTPBad: true})
	if bad >= ok {
		t.Fatalf("bad HTTP status should score lower: ok=%v bad=%v", ok, bad)
	}
}

func TestScoreTLSInvalidPenalty(t *testing.T) {
	ok, _ := Score(Input{Avg: 10, LossPct: 0, Jitter: 1, P95: 20, Samples: 10})
	bad, _ := Score(Input{Avg: 10, LossPct: 0, Jitter: 1, P95: 20, Samples: 10, TLSInvalid: true})
	if bad >= ok {
		t.Fatalf("invalid TLS should score lower: ok=%v bad=%v", ok, bad)
	}
}

func TestGradeThresholds(t *testing.T) {
	cases := []struct {
		score float64
		grade string
	}{
		{100, GradeExcellent}, {90, GradeExcellent}, {89.9, GradeVeryGood},
		{80, GradeVeryGood}, {79.9, GradeGood}, {70, GradeGood},
		{69.9, GradeFair}, {60, GradeFair}, {59.9, GradePoor}, {0, GradePoor},
	}
	for _, c := range cases {
		if got := Grade(c.score); got != c.grade {
			t.Errorf("Grade(%v) = %q, want %q", c.score, got, c.grade)
		}
	}
}

func TestScoreBounds(t *testing.T) {
	// Extreme inputs must stay inside 0-100.
	s, _ := Score(Input{Avg: 100000, LossPct: 100, Jitter: 10000, P95: 99999, Samples: 1, HTTPBad: true, TLSInvalid: true})
	if s < 0 || s > 100 {
		t.Fatalf("score out of bounds: %v", s)
	}
}
