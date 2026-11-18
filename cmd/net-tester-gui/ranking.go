//go:build windows

package main

import (
	"fmt"

	"github.com/erffanhub-00/net-tester/internal/engine"
	. "github.com/lxn/walk/declarative"
)

// rankingTabPage is the leaderboard of the last run: Rank | Target |
// Method | Score | Grade | Avg ms, best first, failed targets last
// as unranked. It shows the same data as the results table, in
// ranked order.
func (a *app) rankingTabPage() TabPage {
	return TabPage{
		Title:  "Ranking",
		Layout: VBox{},
		Children: []Widget{
			// The tab scrolls like the others, so the table stays
			// reachable when the window is small.
			ScrollView{
				Layout: VBox{},
				Children: []Widget{
					TableView{
						AssignTo: &a.rankingView,
						Model:    a.rankingModel,
						Columns:  rankingColumns,
						MinSize:  Size{Width: 0, Height: 300},
					},
					Label{AssignTo: &a.rankingLabel, Text: "Run a test first."},
				},
			},
		},
	}
}

// refreshRanking updates the ranking table from the latest results.
// It must run on the UI thread. An empty (or nil) slice restores
// the empty state.
func (a *app) refreshRanking(results []*engine.Result) {
	a.rankingModel.SetResults(results)
	if len(results) == 0 {
		a.rankingLabel.SetText("Run a test first.")
		return
	}
	ranked := 0
	var best *engine.Result
	for _, r := range results {
		if r.Rank != 0 {
			ranked++
			if best == nil {
				best = r
			}
		}
	}
	if best == nil {
		a.rankingLabel.SetText(fmt.Sprintf("%d targets, all failed.", len(results)))
		return
	}
	a.rankingLabel.SetText(fmt.Sprintf("%d targets ranked — best: %s (score %.1f, %s).",
		ranked, best.Target.Host, best.Score, best.Grade))
}
