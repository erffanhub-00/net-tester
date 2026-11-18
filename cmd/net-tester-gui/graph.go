//go:build windows

package main

import (
	"github.com/lxn/walk"
)

// paintGraph draws the latency graph for the Live Ping tab: a line
// of the last reply latencies, scaled to the largest value. It
// runs on the UI thread when the graph is invalidated.
func (a *app) paintGraph(canvas *walk.Canvas, bounds walk.Rectangle) error {
	a.pingMu.Lock()
	pts := append([]float64(nil), a.pingLatencies...)
	a.pingMu.Unlock()

	if len(pts) < 2 {
		return canvas.DrawText("No data yet — press Start to ping.",
			a.mw.Font(), walk.RGB(128, 128, 128), bounds,
			walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)
	}

	maxV := pts[0]
	for _, v := range pts[1:] {
		if v > maxV {
			maxV = v
		}
	}
	maxV *= 1.1
	if maxV <= 0 {
		maxV = 1
	}

	// Horizontal gridlines at 0, half and full scale.
	gridBrush, err := walk.NewSolidColorBrush(walk.RGB(220, 220, 220))
	if err != nil {
		return err
	}
	defer gridBrush.Dispose()
	gridPen, err := walk.NewGeometricPen(walk.PenSolid, 1, gridBrush)
	if err != nil {
		return err
	}
	defer gridPen.Dispose()
	for _, frac := range []float64{0, 0.5, 1} {
		y := bounds.Y + bounds.Height - int(frac*float64(bounds.Height))
		if err := canvas.DrawLine(gridPen,
			walk.Point{X: bounds.X, Y: y},
			walk.Point{X: bounds.X + bounds.Width, Y: y}); err != nil {
			return err
		}
	}

	// The latency line, teal.
	lineBrush, err := walk.NewSolidColorBrush(walk.RGB(15, 118, 110))
	if err != nil {
		return err
	}
	defer lineBrush.Dispose()
	linePen, err := walk.NewGeometricPen(walk.PenSolid, 2, lineBrush)
	if err != nil {
		return err
	}
	defer linePen.Dispose()

	points := make([]walk.Point, len(pts))
	for i, v := range pts {
		x := bounds.X
		if len(pts) > 1 {
			x += int(float64(i) * float64(bounds.Width) / float64(len(pts)-1))
		}
		y := bounds.Y + bounds.Height - int(v/maxV*float64(bounds.Height)) - 2
		points[i] = walk.Point{X: x, Y: y}
	}
	return canvas.DrawPolyline(linePen, points)
}
