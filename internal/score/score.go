// Package score implements the 0-100 scoring model taken from
// Network-Ping-Pro: higher is better, with letter grades.
//
//	score = 0.40*latency + 0.35*loss + 0.25*jitter
//
// The latency component falls as average latency grows, the loss
// component is 100 minus the loss percentage, and the jitter
// component falls as jitter grows. Two penalties apply on top:
// a slow p95 and a bad HTTP status (or an invalid TLS certificate).
// A target with no successful sample scores 0 and grades Failed.
package score

import "math"

// Input is everything scoring needs about one target.
type Input struct {
	Avg        float64 // average latency in ms
	LossPct    float64 // packet/request loss in percent
	Jitter     float64 // jitter in ms
	P95        float64 // 95th percentile latency in ms
	Samples    int     // successful samples; 0 means the target failed
	HTTPBad    bool    // dominant HTTP status is not 2xx/3xx
	TLSInvalid bool    // TLS certificate did not validate
}

// Grades.
const (
	GradeExcellent = "Excellent"
	GradeVeryGood  = "Very Good"
	GradeGood      = "Good"
	GradeFair      = "Fair"
	GradePoor      = "Poor"
	GradeFailed    = "Failed"
)

// Score computes the 0-100 score and the grade for one target.
func Score(in Input) (float64, string) {
	if in.Samples == 0 {
		return 0, GradeFailed
	}
	lat := clamp(100-in.Avg*0.2, 0, 100)
	loss := clamp(100-in.LossPct, 0, 100)
	jit := clamp(100-in.Jitter*2, 0, 100)
	s := 0.40*lat + 0.35*loss + 0.25*jit
	// Slow tail latency drags the score down.
	switch {
	case in.P95 > 500:
		s *= 0.8
	case in.P95 > 300:
		s *= 0.9
	}
	// The application layer answered with an error status.
	if in.HTTPBad {
		s *= 0.5
	}
	// The certificate does not validate.
	if in.TLSInvalid {
		s *= 0.7
	}
	s = clamp(s, 0, 100)
	s = math.Round(s*10) / 10
	return s, Grade(s)
}

// Grade maps a score to its letter grade. A score of 0 with
// samples still grades Poor, not Failed — Failed is reserved for
// targets with no successful sample at all.
func Grade(score float64) string {
	switch {
	case score >= 90:
		return GradeExcellent
	case score >= 80:
		return GradeVeryGood
	case score >= 70:
		return GradeGood
	case score >= 60:
		return GradeFair
	default:
		return GradePoor
	}
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
