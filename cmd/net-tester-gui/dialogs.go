//go:build windows

package main

import (
	"strings"

	"github.com/erffanhub-00/net-tester/internal/config"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

// targetDialog edits one target. existing is nil when adding.
// It returns the target and true when the user pressed OK.
func (a *app) targetDialog(existing *config.Target) (config.Target, bool) {
	var dlg *walk.Dialog
	var hostEdit *walk.LineEdit
	var portEdit *walk.NumberEdit
	var methodBox *walk.ComboBox

	host, port, method := "", 0, "icmp"
	if existing != nil {
		host, port, method = existing.Host, existing.Port, existing.Method
	}
	result, err := (Dialog{
		Title:    "Target",
		AssignTo: &dlg,
		MinSize:  Size{Width: 340, Height: 260},
		Layout:   VBox{},
		Font:     Font{Family: "Segoe UI", PointSize: 9},
		Children: []Widget{
			Label{Text: "Host:"},
			LineEdit{AssignTo: &hostEdit, Text: host},
			Label{Text: "Port (0 for ping / DNS):"},
			NumberEdit{AssignTo: &portEdit, Value: float64(port), MinValue: 0, MaxValue: 65535},
			Label{Text: "Method:"},
			ComboBox{
				AssignTo:     &methodBox,
				Model:        methodNames,
				CurrentIndex: methodIndex(method),
				Editable:     false,
			},
			Composite{
				Layout: HBox{},
				Children: []Widget{
					HSpacer{},
					PushButton{Text: "OK", OnClicked: func() { dlg.Accept() }},
					PushButton{Text: "Cancel", OnClicked: func() { dlg.Cancel() }},
				},
			},
		},
	}).Run(a.mw)
	if err != nil || result != walk.DlgCmdOK {
		return config.Target{}, false
	}
	t := config.Target{
		Host:   strings.TrimSpace(hostEdit.Text()),
		Port:   int(portEdit.Value()),
		Method: methodNames[methodBox.CurrentIndex()],
	}
	return t, true
}

// categoryDialog edits a category name and description. It returns
// the values and true when the user pressed OK.
func (a *app) categoryDialog(name, desc string) (string, string, bool) {
	var dlg *walk.Dialog
	var nameEdit, descEdit *walk.LineEdit
	result, err := (Dialog{
		Title:    "Category",
		AssignTo: &dlg,
		MinSize:  Size{Width: 340, Height: 220},
		Layout:   VBox{},
		Font:     Font{Family: "Segoe UI", PointSize: 9},
		Children: []Widget{
			Label{Text: "Name:"},
			LineEdit{AssignTo: &nameEdit, Text: name},
			Label{Text: "Description:"},
			LineEdit{AssignTo: &descEdit, Text: desc},
			Composite{
				Layout: HBox{},
				Children: []Widget{
					HSpacer{},
					PushButton{Text: "OK", OnClicked: func() { dlg.Accept() }},
					PushButton{Text: "Cancel", OnClicked: func() { dlg.Cancel() }},
				},
			},
		},
	}).Run(a.mw)
	if err != nil || result != walk.DlgCmdOK {
		return "", "", false
	}
	name = strings.TrimSpace(nameEdit.Text())
	if name == "" {
		showError(a.mw, "A category needs a name.")
		return "", "", false
	}
	return name, strings.TrimSpace(descEdit.Text()), true
}
