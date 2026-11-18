//go:build windows

package main

import (
	"fmt"
	"strings"

	"github.com/erffanhub-00/net-tester/internal/engine"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

// stringListModel is a walk.ListModel over a plain []string.
type stringListModel struct {
	walk.ListModelBase
	items []string
}

func (m *stringListModel) ItemCount() int {
	return len(m.items)
}

func (m *stringListModel) Value(index int) interface{} {
	return m.items[index]
}

// SetItems replaces the items and notifies the list.
func (m *stringListModel) SetItems(items []string) {
	m.items = items
	m.PublishItemsReset()
}

// resultsModel is a walk.TableModel over the run results. Rows are
// kept in the engine's ranked order; Value returns pre-formatted
// strings so no column sorting support is needed.
type resultsModel struct {
	walk.TableModelBase
	results []*engine.Result
}

func (m *resultsModel) RowCount() int {
	return len(m.results)
}

func (m *resultsModel) Value(row, col int) interface{} {
	if row < 0 || row >= len(m.results) {
		return ""
	}
	r := m.results[row]
	switch col {
	case 0:
		return targetLabel(r.Target)
	case 1:
		return r.Target.Method
	case 2:
		return r.DisplayStatus()
	case 3:
		return fmt.Sprintf("%.1f", r.Score)
	case 4:
		return fmt.Sprintf("%.1f", r.Avg)
	case 5:
		return fmt.Sprintf("%.1f%%", r.LossPct)
	case 6:
		return fmt.Sprintf("%.1f", r.Jitter)
	case 7:
		return fmt.Sprintf("%.1f", r.P50)
	case 8:
		return fmt.Sprintf("%.1f", r.P95)
	case 9:
		if r.Note != "" {
			return r.Note
		}
		return r.Error
	}
	return ""
}

// SetResults replaces the rows and notifies the table.
func (m *resultsModel) SetResults(results []*engine.Result) {
	m.results = results
	m.PublishRowsReset()
}

// resultColumns are the results table columns.
var resultColumns = []TableViewColumn{
	{Title: "Target", Width: 220},
	{Title: "Method", Width: 70},
	{Title: "Status", Width: 90},
	{Title: "Score", Width: 55},
	{Title: "Avg ms", Width: 70},
	{Title: "Loss %", Width: 60},
	{Title: "Jitter", Width: 60},
	{Title: "P50", Width: 60},
	{Title: "P95", Width: 60},
	{Title: "Note", Width: 260},
}

// summaryLine renders the one-line run summary for the status label.
func summaryLine(results []*engine.Result) string {
	total := len(results)
	failed := 0
	var best *engine.Result
	for _, r := range results {
		if r.Failed() {
			failed++
			continue
		}
		if best == nil || r.Score > best.Score {
			best = r
		}
	}
	s := fmt.Sprintf("%d targets tested, %d OK, %d failed.", total, total-failed, failed)
	if best != nil {
		s += fmt.Sprintf(" Top ranked: %s (score %.1f, %s).", best.Target.Host, best.Score, best.Grade)
	}
	return s
}

// methodNames are the methods a target can use.
var methodNames = []string{"icmp", "tcp", "http", "https", "dns"}

func methodIndex(method string) int {
	for i, m := range methodNames {
		if m == strings.ToLower(method) {
			return i
		}
	}
	return 0
}
