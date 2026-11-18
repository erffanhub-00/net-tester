//go:build windows

package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/erffanhub-00/net-tester/internal/config"
	"github.com/erffanhub-00/net-tester/internal/engine"
	"github.com/erffanhub-00/net-tester/internal/report"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

func (a *app) testTabPage() TabPage {
	return TabPage{
		Title:  "Test",
		Layout: VBox{},
		Children: []Widget{
			// The whole tab scrolls, so every control stays
			// reachable when the window is small or the display
			// is scaled. The results table keeps its own
			// scrollbar for long runs; it gets a fixed viewport
			// height here because stretch is ignored inside a
			// scroll view.
			ScrollView{
				Layout: VBox{},
				Children: []Widget{
					Composite{
						Layout: HBox{},
						Children: []Widget{
							GroupBox{
								Title:  "Categories",
								Layout: VBox{},
								Children: []Widget{
									ListBox{
										AssignTo:       &a.catList,
										Model:          a.catModel,
										MultiSelection: true,
										MinSize:        Size{Width: 200, Height: 120},
										OnCurrentIndexChanged: func() {
											a.refreshTargets()
										},
									},
									Composite{
										Layout: HBox{},
										Children: []Widget{
											PushButton{Text: "Add category", OnClicked: func() { a.addCategory() }},
											PushButton{Text: "Delete", OnClicked: func() { a.deleteCategory() }},
										},
									},
								},
							},
							GroupBox{
								Title:  "Targets",
								Layout: VBox{},
								Children: []Widget{
									ListBox{
										AssignTo: &a.tgtList,
										Model:    a.tgtModel,
										MinSize:  Size{Width: 300, Height: 120},
									},
									Composite{
										Layout: HBox{},
										Children: []Widget{
											PushButton{Text: "Add target", OnClicked: func() { a.addTarget() }},
											PushButton{Text: "Edit", OnClicked: func() { a.editTarget() }},
											PushButton{Text: "Delete", OnClicked: func() { a.deleteTarget() }},
											PushButton{Text: "Import...", OnClicked: func() { a.importDialog() }},
										},
									},
								},
							},
						},
					},
					GroupBox{
						Title:  "Settings",
						Layout: Grid{Columns: 10},
						Children: []Widget{
							Label{Text: "Packets:"},
							// NOTE: walk's NumberEdit Value is a Property
							// (interface{}), and its setter only accepts
							// float64 — an int literal silently becomes 0,
							// which is out of range and crashes window
							// creation ("value out of range"). So every
							// Value here must be float64, not a bare int.
							NumberEdit{AssignTo: &a.packetsEdit, Value: float64(10), MinValue: 1, MaxValue: 200},
							Label{Text: "Workers:"},
							NumberEdit{AssignTo: &a.workersEdit, Value: float64(10), MinValue: 1, MaxValue: 50},
							Label{Text: "Ping timeout (ms):"},
							NumberEdit{AssignTo: &a.pingTimeoutEdit, Value: float64(2000), MinValue: 100, MaxValue: 60000},
							Label{Text: "TCP timeout (ms):"},
							NumberEdit{AssignTo: &a.tcpTimeoutEdit, Value: float64(3000), MinValue: 100, MaxValue: 60000},
							Label{Text: "HTTP timeout (ms):"},
							NumberEdit{AssignTo: &a.httpTimeoutEdit, Value: float64(5000), MinValue: 100, MaxValue: 60000},
						},
					},
					Composite{
						Layout: HBox{},
						Children: []Widget{
							PushButton{AssignTo: &a.runBtn, Text: "Run", OnClicked: func() { a.onRun() }},
							PushButton{AssignTo: &a.stopBtn, Text: "Stop", Enabled: false, OnClicked: func() { a.onStop() }},
							PushButton{AssignTo: &a.exportBtn, Text: "Export CSV + JSON...", OnClicked: func() { a.onExport() }},
							HSpacer{},
						},
					},
					TableView{
						AssignTo:      &a.resultsView,
						Model:         a.resultsModel,
						Columns:       resultColumns,
						MinSize:       Size{Width: 0, Height: 260},
						StretchFactor: 1,
					},
					Label{AssignTo: &a.statusLabel, Text: "Select one or more categories and press Run."},
					Label{AssignTo: &a.netLabel, Text: ""},
				},
			},
		},
	}
}

// refreshTargets shows the targets of the currently selected
// category in the Targets list.
func (a *app) refreshTargets() {
	idx := a.catList.CurrentIndex()
	if idx < 0 || idx >= len(a.cfg.Categories) {
		a.tgtModel.SetItems(nil)
		return
	}
	items := make([]string, 0, len(a.cfg.Categories[idx].Targets))
	for _, t := range a.cfg.Categories[idx].Targets {
		items = append(items, targetLabel(t))
	}
	a.tgtModel.SetItems(items)
}

// selectedCategories returns the categories the user picked in the
// list (all of them when nothing is selected, so Run never does
// nothing by accident — except the button asks first).
func (a *app) selectedCategories() []config.Category {
	idxs := a.catList.SelectedIndexes()
	if len(idxs) == 0 {
		return nil
	}
	out := make([]config.Category, 0, len(idxs))
	for _, i := range idxs {
		if i >= 0 && i < len(a.cfg.Categories) {
			out = append(out, a.cfg.Categories[i])
		}
	}
	return out
}

func (a *app) setRunning(running bool) {
	a.running = running
	a.runBtn.SetEnabled(!running)
	a.stopBtn.SetEnabled(running)
}

func (a *app) onRun() {
	if a.running {
		return
	}
	cats := a.selectedCategories()
	if len(cats) == 0 {
		showInfo(a.mw, "Select at least one category first.")
		return
	}
	var targets []config.Target
	var names []string
	for _, c := range cats {
		targets = append(targets, c.Targets...)
		names = append(names, c.Name)
	}
	a.exportCats = strings.Join(names, "+")
	opts := a.optionsFromUI()
	a.results = nil
	a.resultsModel.SetResults(nil)
	a.refreshRanking(nil)
	a.statusLabel.SetText(fmt.Sprintf("Testing %d targets...", len(targets)))
	a.setRunning(true)

	ctx, cancel := context.WithCancel(context.Background())
	a.runCancel = cancel
	a.runSeq++
	seq := a.runSeq
	go a.runTargets(ctx, seq, targets, opts)
}

// runTargets tests every target in parallel (up to Workers at a
// time) and streams each finished result to the table. It runs on a
// background goroutine; all UI updates go through Synchronize.
func (a *app) runTargets(ctx context.Context, seq int, targets []config.Target, o engine.Options) {
	seen := map[string]bool{}
	unique := make([]config.Target, 0, len(targets))
	for _, t := range targets {
		if seen[t.Key()] {
			continue
		}
		seen[t.Key()] = true
		unique = append(unique, t)
	}
	sem := make(chan struct{}, o.Workers)
	var wg sync.WaitGroup
	var mu sync.Mutex
	done := 0
	for _, t := range unique {
		wg.Add(1)
		go func(t config.Target) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			r := engine.RunOne(ctx, t, o)
			mu.Lock()
			a.results = append(a.results, r)
			engine.RankResults(a.results)
			done++
			finished, total := done, len(unique)
			snap := append([]*engine.Result(nil), a.results...)
			mu.Unlock()
			a.mw.Synchronize(func() {
				if seq != a.runSeq {
					return
				}
				a.resultsModel.SetResults(snap)
				a.refreshRanking(snap)
				a.statusLabel.SetText(fmt.Sprintf("Testing... %d/%d done.", finished, total))
			})
		}(t)
	}
	wg.Wait()
	engine.ApplyICMPHints(a.results)
	engine.RankResults(a.results)
	final := append([]*engine.Result(nil), a.results...)
	a.mw.Synchronize(func() {
		if seq != a.runSeq {
			return
		}
		a.resultsModel.SetResults(final)
		a.refreshRanking(final)
		a.setRunning(false)
		a.statusLabel.SetText(summaryLine(final))
	})
}

func (a *app) onStop() {
	if a.runCancel != nil {
		a.runCancel()
	}
	a.statusLabel.SetText("Stopping...")
}

// onExport writes CSV + JSON of the current results, only when the
// user asks: the GUI never exports on its own.
func (a *app) onExport() {
	if len(a.results) == 0 {
		showInfo(a.mw, "Nothing to export yet — run a test first.")
		return
	}
	dlg := walk.FileDialog{
		Title:    "Export results",
		Filter:   "CSV files (*.csv)|*.csv",
		FilePath: "net-tester_results.csv",
	}
	ok, err := dlg.ShowSave(a.mw)
	if err != nil {
		showError(a.mw, "Could not open the save dialog: "+err.Error())
		return
	}
	if !ok {
		return
	}
	csvPath := dlg.FilePath
	if !strings.HasSuffix(strings.ToLower(csvPath), ".csv") {
		csvPath += ".csv"
	}
	jsonPath := strings.TrimSuffix(csvPath, filepath.Ext(csvPath)) + ".json"
	if err := report.WriteCSV(csvPath, a.results); err != nil {
		showError(a.mw, "Could not write the CSV file: "+err.Error())
		return
	}
	if err := report.WriteJSON(jsonPath, a.exportCats, a.results, a.nc); err != nil {
		showError(a.mw, "Could not write the JSON file: "+err.Error())
		return
	}
	a.statusLabel.SetText(fmt.Sprintf("Exported: %s and %s", csvPath, jsonPath))
}

// -- Category editing --

func (a *app) addCategory() {
	name, desc, ok := a.categoryDialog("", "")
	if !ok {
		return
	}
	for _, c := range a.cfg.Categories {
		if strings.EqualFold(c.Name, name) {
			showError(a.mw, fmt.Sprintf("A category named %q already exists.", name))
			return
		}
	}
	a.cfg.Categories = append(a.cfg.Categories, config.Category{Name: name, Description: desc, Targets: []config.Target{}})
	if !a.saveConfig() {
		a.cfg.Categories = a.cfg.Categories[:len(a.cfg.Categories)-1]
		return
	}
	a.refreshCategories()
	a.catList.SetSelectedIndexes([]int{len(a.cfg.Categories) - 1})
}

func (a *app) deleteCategory() {
	idx := a.catList.CurrentIndex()
	if idx < 0 || idx >= len(a.cfg.Categories) {
		showInfo(a.mw, "Select a category to delete.")
		return
	}
	if len(a.cfg.Categories) == 1 {
		showError(a.mw, "Cannot delete the last category.")
		return
	}
	name := a.cfg.Categories[idx].Name
	if walk.MsgBox(a.mw, "net-tester", fmt.Sprintf("Delete category %q and its %d targets?", name, len(a.cfg.Categories[idx].Targets)),
		walk.MsgBoxOKCancel|walk.MsgBoxIconWarning) != walk.DlgCmdOK {
		return
	}
	removed := a.cfg.Categories[idx]
	a.cfg.Categories = append(a.cfg.Categories[:idx], a.cfg.Categories[idx+1:]...)
	if !a.saveConfig() {
		// Restore the in-memory state: the file was not changed.
		a.cfg.Categories = append(a.cfg.Categories, config.Category{})
		copy(a.cfg.Categories[idx+1:], a.cfg.Categories[idx:])
		a.cfg.Categories[idx] = removed
		return
	}
	a.refreshCategories()
}

// -- Target editing --

func (a *app) currentCategory() (int, *config.Category) {
	idx := a.catList.CurrentIndex()
	if idx < 0 || idx >= len(a.cfg.Categories) {
		return -1, nil
	}
	return idx, &a.cfg.Categories[idx]
}

func (a *app) addTarget() {
	_, cat := a.currentCategory()
	if cat == nil {
		showInfo(a.mw, "Select a category first.")
		return
	}
	t, ok := a.targetDialog(nil)
	if !ok {
		return
	}
	if err := config.ValidateTarget(&t); err != nil {
		showError(a.mw, "Invalid target: "+err.Error())
		return
	}
	for _, existing := range cat.Targets {
		if existing.Key() == t.Key() {
			showError(a.mw, "That target is already in this category.")
			return
		}
	}
	cat.Targets = append(cat.Targets, t)
	if !a.saveConfig() {
		cat.Targets = cat.Targets[:len(cat.Targets)-1]
		return
	}
	a.refreshTargets()
}

func (a *app) editTarget() {
	_, cat := a.currentCategory()
	if cat == nil {
		showInfo(a.mw, "Select a category first.")
		return
	}
	idx := a.tgtList.CurrentIndex()
	if idx < 0 || idx >= len(cat.Targets) {
		showInfo(a.mw, "Select a target to edit.")
		return
	}
	orig := cat.Targets[idx]
	t, ok := a.targetDialog(&orig)
	if !ok {
		return
	}
	if err := config.ValidateTarget(&t); err != nil {
		showError(a.mw, "Invalid target: "+err.Error())
		return
	}
	for i, existing := range cat.Targets {
		if i != idx && existing.Key() == t.Key() {
			showError(a.mw, "That target is already in this category.")
			return
		}
	}
	cat.Targets[idx] = t
	if !a.saveConfig() {
		cat.Targets[idx] = orig
		return
	}
	a.refreshTargets()
}

func (a *app) deleteTarget() {
	_, cat := a.currentCategory()
	if cat == nil {
		showInfo(a.mw, "Select a category first.")
		return
	}
	idx := a.tgtList.CurrentIndex()
	if idx < 0 || idx >= len(cat.Targets) {
		showInfo(a.mw, "Select a target to delete.")
		return
	}
	if len(cat.Targets) == 1 {
		showError(a.mw, "A category needs at least one target.")
		return
	}
	removed := cat.Targets[idx]
	cat.Targets = append(cat.Targets[:idx], cat.Targets[idx+1:]...)
	if !a.saveConfig() {
		// Restore the in-memory state: the file was not changed.
		cat.Targets = append(cat.Targets, config.Target{})
		copy(cat.Targets[idx+1:], cat.Targets[idx:])
		cat.Targets[idx] = removed
		return
	}
	a.refreshTargets()
}
