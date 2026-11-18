//go:build windows

// Command net-tester-gui is the graphical interface for net-tester:
// a live results table, in-app settings and category/target editing,
// export on demand, a live ping monitor and a ranking table.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/erffanhub-00/net-tester/internal/config"
	"github.com/erffanhub-00/net-tester/internal/engine"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

const version = "2.2.0"

type app struct {
	mw      *walk.MainWindow
	cfg     *config.File
	cfgPath string
	nc      engine.NetworkContext

	// Test tab.
	catModel        *stringListModel
	catList         *walk.ListBox
	tgtModel        *stringListModel
	tgtList         *walk.ListBox
	packetsEdit     *walk.NumberEdit
	workersEdit     *walk.NumberEdit
	pingTimeoutEdit *walk.NumberEdit
	tcpTimeoutEdit  *walk.NumberEdit
	httpTimeoutEdit *walk.NumberEdit
	runBtn          *walk.PushButton
	stopBtn         *walk.PushButton
	exportBtn       *walk.PushButton
	resultsModel    *resultsModel
	resultsView     *walk.TableView
	statusLabel     *walk.Label
	netLabel        *walk.Label

	runSeq     int
	runCancel  context.CancelFunc
	running    bool
	results    []*engine.Result
	exportCats string

	// Ranking tab.
	rankingModel *rankingModel
	rankingView  *walk.TableView
	rankingLabel *walk.Label

	// Live Ping tab.
	pingHostEdit   *walk.LineEdit
	pingStartBtn   *walk.PushButton
	pingStopBtn    *walk.PushButton
	pingCurrentLbl *walk.Label
	pingNoteLabel  *walk.Label
	pingStatsLbl   *walk.Label
	pingListModel  *stringListModel
	pingList       *walk.ListBox
	pingGraph      *walk.CustomWidget
	pingCancel     context.CancelFunc
	pingRunning    bool
	pingMu         sync.Mutex
	pingStats      engine.LivePingStats
	pingLines      []string
	pingLatencies  []float64 // last 60 reply latencies, ms
}

func main() {
	a := &app{}
	if err := a.init(); err != nil {
		walk.MsgBox(nil, "net-tester", "Cannot start: "+err.Error(), walk.MsgBoxOK|walk.MsgBoxIconError)
		os.Exit(1)
	}
	if err := a.createWindow(); err != nil {
		walk.MsgBox(nil, "net-tester", "Cannot create window: "+err.Error(), walk.MsgBoxOK|walk.MsgBoxIconError)
		os.Exit(1)
	}
	a.mw.Run()
}

func (a *app) init() error {
	a.cfgPath = configPath()
	cfg, err := loadConfigFile(a.cfgPath)
	if err != nil {
		return err
	}
	a.cfg = cfg
	a.nc = engine.DetectNetworkContext("")
	a.catModel = &stringListModel{items: a.cfg.Names()}
	a.tgtModel = &stringListModel{}
	a.resultsModel = &resultsModel{}
	a.rankingModel = &rankingModel{}
	a.pingListModel = &stringListModel{}
	return nil
}

// configPath is categories.json next to the exe; it falls back to
// the working directory when the exe path cannot be determined.
func configPath() string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), "categories.json")
	}
	return "categories.json"
}

func loadConfigFile(path string) (*config.File, error) {
	if _, err := os.Stat(path); err == nil {
		return config.Load(path)
	}
	return config.Seed()
}

// saveConfig writes the in-memory categories back to disk and
// reports failures in a dialog. It returns false on failure.
func (a *app) saveConfig() bool {
	if err := config.Save(a.cfgPath, a.cfg); err != nil {
		walk.MsgBox(a.mw, "net-tester", "Could not save categories.json: "+err.Error(), walk.MsgBoxOK|walk.MsgBoxIconError)
		return false
	}
	return true
}

func (a *app) createWindow() error {
	var mw *walk.MainWindow
	err := (MainWindow{
		AssignTo: &mw,
		Title:    "net-tester " + version,
		MinSize:  Size{Width: 980, Height: 640},
		Size:     Size{Width: 980, Height: 640},
		Layout:   VBox{},
		Font:     Font{Family: "Segoe UI", PointSize: 9},
		Children: []Widget{
			TabWidget{
				Pages: []TabPage{
					a.testTabPage(),
					a.livePingTabPage(),
					a.rankingTabPage(),
				},
			},
		},
	}).Create()
	if err != nil {
		return err
	}
	a.mw = mw
	a.refreshCategories()
	a.refreshNetLabel()
	if a.nc.ProxyActive() {
		a.pingNoteLabel.SetText("Note: " + a.nc.Notes()[len(a.nc.Notes())-1])
	}
	return nil
}

// refreshCategories rebuilds the category list from the config.
func (a *app) refreshCategories() {
	a.catModel.SetItems(a.cfg.Names())
	if len(a.cfg.Categories) > 0 {
		a.catList.SetSelectedIndexes([]int{0})
	}
	a.refreshTargets()
}

// refreshNetLabel shows the network-context notes (proxy / VPN)
// under the results table, or nothing when neither is present.
func (a *app) refreshNetLabel() {
	a.netLabel.SetText(strings.Join(a.nc.Notes(), " "))
}

// optionsFromUI builds the run options from the settings fields,
// clamping to the valid ranges.
func (a *app) optionsFromUI() engine.Options {
	o := engine.DefaultOptions()
	o.Packets = clampInt(int(a.packetsEdit.Value()), 1, 200)
	o.Workers = clampInt(int(a.workersEdit.Value()), 1, 50)
	o.PingTimeout = msToDuration(a.pingTimeoutEdit.Value())
	o.TCPTimeout = msToDuration(a.tcpTimeoutEdit.Value())
	o.HTTPTimeout = msToDuration(a.httpTimeoutEdit.Value())
	return o
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func msToDuration(v float64) time.Duration {
	ms := int(v)
	if ms < 100 {
		ms = 100
	}
	if ms > 60000 {
		ms = 60000
	}
	return time.Duration(ms) * time.Millisecond
}

func targetLabel(t config.Target) string {
	name := t.Host
	if t.Port != 0 {
		name += ":" + fmt.Sprintf("%d", t.Port)
	}
	return name + "/" + t.Method
}

func showError(owner walk.Form, msg string) {
	walk.MsgBox(owner, "net-tester", msg, walk.MsgBoxOK|walk.MsgBoxIconError)
}

func showInfo(owner walk.Form, msg string) {
	walk.MsgBox(owner, "net-tester", msg, walk.MsgBoxOK|walk.MsgBoxIconInformation)
}
