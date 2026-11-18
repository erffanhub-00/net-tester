//go:build windows

package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/erffanhub-00/net-tester/internal/engine"
	. "github.com/lxn/walk/declarative"
)

const (
	maxPingLines     = 50 // replies kept in the list
	maxPingLatencies = 60 // samples kept for the graph
)

func (a *app) livePingTabPage() TabPage {
	return TabPage{
		Title:  "Live Ping",
		Layout: VBox{},
		Children: []Widget{
			Composite{
				Layout: HBox{},
				Children: []Widget{
					Label{Text: "Host:"},
					LineEdit{AssignTo: &a.pingHostEdit, Text: "google.com"},
					PushButton{AssignTo: &a.pingStartBtn, Text: "Start", OnClicked: func() { a.onPingStart() }},
					PushButton{AssignTo: &a.pingStopBtn, Text: "Stop", Enabled: false, OnClicked: func() { a.onPingStop() }},
					HSpacer{},
				},
			},
			Label{AssignTo: &a.pingNoteLabel, Text: ""},
			Label{AssignTo: &a.pingStatsLbl, Text: "Not running."},
			CustomWidget{
				AssignTo: &a.pingGraph,
				MinSize:  Size{Width: 0, Height: 180},
				Paint:    a.paintGraph,
			},
			ListBox{
				AssignTo:      &a.pingList,
				Model:         a.pingListModel,
				MinSize:       Size{Width: 0, Height: 140},
				StretchFactor: 1,
			},
		},
	}
}

func (a *app) onPingStart() {
	if a.pingRunning {
		return
	}
	host := strings.TrimSpace(a.pingHostEdit.Text())
	if host == "" {
		showInfo(a.mw, "Enter a host to ping.")
		return
	}
	a.pingMu.Lock()
	a.pingStats = engine.LivePingStats{}
	a.pingLines = nil
	a.pingLatencies = nil
	a.pingMu.Unlock()
	a.pingListModel.SetItems(nil)
	a.pingStatsLbl.SetText("Pinging " + host + "...")
	a.pingStartBtn.SetEnabled(false)
	a.pingStopBtn.SetEnabled(true)
	a.pingRunning = true

	ctx, cancel := context.WithCancel(context.Background())
	a.pingCancel = cancel
	go func() {
		err := engine.StreamPing(ctx, host, func(line string) {
			a.onPingLine(line)
		})
		a.mw.Synchronize(func() {
			a.pingRunning = false
			a.pingStartBtn.SetEnabled(true)
			a.pingStopBtn.SetEnabled(false)
			if err != nil && ctx.Err() == nil {
				a.pingStatsLbl.SetText("Ping failed: " + err.Error())
			}
		})
	}()
}

func (a *app) onPingStop() {
	if a.pingCancel != nil {
		a.pingCancel()
	}
}

// onPingLine handles one line of ping output: it updates the stats,
// the reply list and the graph. It runs on the ping reader
// goroutine; UI updates go through Synchronize.
func (a *app) onPingLine(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	a.pingMu.Lock()
	if ms, ok := engine.ParsePingReplyLine(line); ok {
		a.pingStats.AddReply(ms)
		a.pingLatencies = append(a.pingLatencies, ms)
		if len(a.pingLatencies) > maxPingLatencies {
			a.pingLatencies = a.pingLatencies[len(a.pingLatencies)-maxPingLatencies:]
		}
	} else if isPingMissLine(line) {
		a.pingStats.AddMiss()
	}
	a.pingLines = append(a.pingLines, line)
	if len(a.pingLines) > maxPingLines {
		a.pingLines = a.pingLines[len(a.pingLines)-maxPingLines:]
	}
	lines := append([]string(nil), a.pingLines...)
	stats := a.pingStats
	a.pingMu.Unlock()

	a.mw.Synchronize(func() {
		a.pingListModel.SetItems(lines)
		if len(lines) > 0 {
			_ = a.pingList.SetCurrentIndex(len(lines) - 1)
		}
		a.pingStatsLbl.SetText(formatPingStats(&stats))
		_ = a.pingGraph.Invalidate()
	})
}

// isPingMissLine reports whether a ping output line is a failed
// reply slot rather than a header or statistics line: Windows
// prints "Request timed out." and "Destination host unreachable."
// for those.
func isPingMissLine(line string) bool {
	lower := strings.ToLower(line)
	return strings.Contains(lower, "timed out") ||
		strings.Contains(lower, "unreachable")
}

// formatPingStats renders the live stats line.
func formatPingStats(s *engine.LivePingStats) string {
	if s.Sent == 0 {
		return "No replies yet."
	}
	sum := s.Summary()
	return fmt.Sprintf("Sent %d, received %d, loss %.1f%% — avg %.1f ms, min %.1f, max %.1f, jitter %.1f",
		s.Sent, s.Received, s.LossPct(), sum.Avg, sum.Min, sum.Max, sum.Jitter)
}
