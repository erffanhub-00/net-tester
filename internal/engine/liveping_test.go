package engine

import "testing"

func TestParsePingReplyLine(t *testing.T) {
	cases := []struct {
		line string
		ms   float64
		ok   bool
	}{
		{"Reply from 8.8.8.8: bytes=32 time=12ms TTL=117", 12, true},
		{"Reply from 8.8.8.8: bytes=32 time<1ms TTL=117", 0.5, true},
		{"64 bytes from 8.8.8.8: icmp_seq=1 ttl=117 time=11.2 ms", 11.2, true},
		{"64 bytes from 8.8.8.8: icmp_seq=0 ttl=117 time=11.234 ms", 11.234, true},
		{"Request timed out.", 0, false},
		{"Reply from 10.0.0.5: Destination host unreachable.", 0, false},
		{"Ping statistics for 8.8.8.8:", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		ms, ok := ParsePingReplyLine(c.line)
		if ok != c.ok || ms != c.ms {
			t.Errorf("ParsePingReplyLine(%q) = (%v, %v), want (%v, %v)", c.line, ms, ok, c.ms, c.ok)
		}
	}
}

func TestLivePingStats(t *testing.T) {
	var s LivePingStats
	s.AddReply(10)
	s.AddReply(20)
	s.AddMiss()
	if s.Sent != 3 || s.Received != 2 {
		t.Fatalf("sent=%d received=%d, want 3/2", s.Sent, s.Received)
	}
	if got := s.LossPct(); got < 33.3 || got > 33.4 {
		t.Fatalf("loss = %v, want ~33.33", got)
	}
	sum := s.Summary()
	if sum.Avg != 15 || sum.Min != 10 || sum.Max != 20 {
		t.Fatalf("avg/min/max = %v/%v/%v, want 15/10/20", sum.Avg, sum.Min, sum.Max)
	}
	if got := (&LivePingStats{}).LossPct(); got != 0 {
		t.Fatalf("empty stats loss = %v, want 0", got)
	}
}
