//go:build windows

package main

import (
	"testing"

	"github.com/erffanhub-00/net-tester/internal/config"
	"github.com/erffanhub-00/net-tester/internal/engine"
)

// TestRankingModel checks the ranking table's cell mapping,
// including the "—" rank for failed (unranked) targets.
func TestRankingModel(t *testing.T) {
	m := &rankingModel{}
	m.SetResults([]*engine.Result{
		{Target: config.Target{Host: "a.example", Port: 443, Method: "https"}, Score: 95, Grade: "Excellent", Rank: 1, Avg: 10.5},
		{Target: config.Target{Host: "b.example", Port: 0, Method: "icmp"}, Score: 0, Grade: "Failed", Rank: 0, Avg: 0},
	})
	if m.RowCount() != 2 {
		t.Fatalf("RowCount = %d, want 2", m.RowCount())
	}
	cases := []struct {
		row, col int
		want     string
	}{
		{0, 0, "1"},
		{1, 0, "—"},
		{0, 1, "a.example:443/https"},
		{0, 2, "https"},
		{0, 3, "95.0"},
		{0, 4, "Excellent"},
		{1, 4, "Failed"},
		{0, 5, "10.5"},
		{5, 0, ""},
	}
	for _, c := range cases {
		if got := m.Value(c.row, c.col); got != c.want {
			t.Errorf("Value(%d,%d) = %v, want %q", c.row, c.col, got, c.want)
		}
	}
}
