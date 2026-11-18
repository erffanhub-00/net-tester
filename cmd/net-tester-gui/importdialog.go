//go:build windows

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/erffanhub-00/net-tester/internal/config"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

// configCopy deep-copies the categories so an import can be built
// on the copy and only applied after the file saves successfully.
func (a *app) configCopy() *config.File {
	cp := &config.File{Categories: make([]config.Category, len(a.cfg.Categories))}
	for i, c := range a.cfg.Categories {
		cp.Categories[i] = c
		cp.Categories[i].Targets = append([]config.Target(nil), c.Targets...)
	}
	return cp
}

// importSummary renders the result of an import for the user.
func importSummary(added int, dest string, skipped []config.ImportSkip) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Added %d target(s) to %q.", added, dest)
	if len(skipped) > 0 {
		fmt.Fprintf(&b, "\nSkipped %d:", len(skipped))
		for i, s := range skipped {
			if i >= 10 {
				fmt.Fprintf(&b, "\n...and %d more", len(skipped)-10)
				break
			}
			fmt.Fprintf(&b, "\n%s — %s", s.Line, s.Reason)
		}
	}
	return b.String()
}

// importDialog opens the bulk-import dialog: paste targets (one per
// line) or import them from a file. Targets go to the category the
// user picks (or types, to create a new one) and are saved to
// categories.json. A .json file is a categories file and is merged
// instead (new categories added, new targets appended).
func (a *app) importDialog() {
	var dlg *walk.Dialog
	var catBox *walk.ComboBox
	var textEdit *walk.TextEdit

	_, curCat := a.currentCategory()
	curName := ""
	curIdx := 0
	if curCat != nil {
		curName = curCat.Name
		for i, c := range a.cfg.Categories {
			if c.Name == curName {
				curIdx = i
				break
			}
		}
	}

	jsonImported := false
	result, err := (Dialog{
		Title:    "Import targets",
		AssignTo: &dlg,
		MinSize:  Size{Width: 480, Height: 460},
		Layout:   VBox{},
		Font:     Font{Family: "Segoe UI", PointSize: 9},
		Children: []Widget{
			Label{Text: "Category (pick one or type a new name):"},
			ComboBox{
				AssignTo:     &catBox,
				Model:        a.cfg.Names(),
				Editable:     true,
				CurrentIndex: curIdx,
			},
			Label{Text: "Targets — one per line: host, host:port, host method, host:port method"},
			Label{Text: "Examples: example.com   1.1.1.1:53 dns   google.com https"},
			TextEdit{
				AssignTo: &textEdit,
				MinSize:  Size{Width: 0, Height: 180},
			},
			Composite{
				Layout: HBox{},
				Children: []Widget{
					PushButton{Text: "Import from file...", OnClicked: func() {
						a.importFromFile(dlg, textEdit, &jsonImported)
					}},
					HSpacer{},
					PushButton{Text: "Import", OnClicked: func() { dlg.Accept() }},
					PushButton{Text: "Cancel", OnClicked: func() { dlg.Cancel() }},
				},
			},
		},
	}).Run(a.mw)
	if err != nil || result != walk.DlgCmdOK || jsonImported {
		return
	}

	dest := strings.TrimSpace(catBox.Text())
	if dest == "" {
		showError(a.mw, "Pick a category or type a new name.")
		return
	}
	text := textEdit.Text()
	if strings.TrimSpace(text) == "" {
		showInfo(a.mw, "Nothing to import — paste some targets first.")
		return
	}
	a.importText(dest, text)
}

// importText parses pasted lines into the destination category
// (creating it when it does not exist yet) and saves.
func (a *app) importText(dest, text string) {
	candidate := a.configCopy()
	cat, ok := candidate.Find(dest)
	var existing []config.Target
	if ok {
		existing = cat.Targets
	}
	added, skipped := config.ParseImport(text, existing)
	if len(added) == 0 && len(skipped) == 0 {
		showInfo(a.mw, "Nothing to import — paste some targets first.")
		return
	}
	if !ok {
		candidate.Categories = append(candidate.Categories, config.Category{Name: dest, Description: "Imported targets"})
		cat, _ = candidate.Find(dest)
	}
	cat.Targets = append(cat.Targets, added...)
	if err := config.Save(a.cfgPath, candidate); err != nil {
		showError(a.mw, "Could not save categories.json: "+err.Error())
		return
	}
	a.cfg = candidate
	a.refreshCategories()
	for i, c := range a.cfg.Categories {
		if c.Name == dest {
			a.catList.SetSelectedIndexes([]int{i})
			break
		}
	}
	a.refreshTargets()
	showInfo(a.mw, importSummary(len(added), dest, skipped))
}

// importFromFile handles the dialog's "Import from file..." button.
// A .json file is a categories file and is merged right away (the
// dialog then closes); a .txt file is loaded into the text box for
// the user to import with the Import button. jsonImported is set
// when a JSON merge already happened, so the caller skips the
// pasted-text import.
func (a *app) importFromFile(dlg *walk.Dialog, textEdit *walk.TextEdit, jsonImported *bool) {
	fd := walk.FileDialog{
		Title:  "Import targets from file",
		Filter: "Categories JSON (*.json)|*.json|Text files (*.txt)|*.txt|All files (*.*)|*.*",
	}
	ok, err := fd.ShowOpen(a.mw)
	if err != nil {
		showError(a.mw, "Could not open the file dialog: "+err.Error())
		return
	}
	if !ok {
		return
	}
	data, err := os.ReadFile(fd.FilePath)
	if err != nil {
		showError(a.mw, "Could not read the file: "+err.Error())
		return
	}
	if strings.HasSuffix(strings.ToLower(fd.FilePath), ".json") {
		a.importJSONFile(dlg, data, jsonImported)
		return
	}
	textEdit.SetText(string(data))
}

// importJSONFile merges a categories file into the config and
// closes the dialog.
func (a *app) importJSONFile(dlg *walk.Dialog, data []byte, jsonImported *bool) {
	imported, err := config.Parse(data)
	if err != nil {
		showError(a.mw, "Invalid categories file: "+err.Error())
		return
	}
	candidate := a.configCopy()
	addedCat, addedTgt, skipped := config.MergeFile(candidate, imported)
	if err := config.Save(a.cfgPath, candidate); err != nil {
		showError(a.mw, "Could not save categories.json: "+err.Error())
		return
	}
	a.cfg = candidate
	a.refreshCategories()
	*jsonImported = true
	showInfo(a.mw, fmt.Sprintf("Imported %d new categor%s and %d target(s) (%d duplicate(s) skipped).",
		addedCat, pluralS(addedCat, "y", "ies"), addedTgt, skipped))
	dlg.Accept()
}

func pluralS(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}
