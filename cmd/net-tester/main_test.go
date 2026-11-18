package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erffanhub-00/net-tester/internal/config"
	"github.com/erffanhub-00/net-tester/internal/engine"
)

func TestParseExportFlag(t *testing.T) {
	cases := []struct {
		in       string
		wantCSV  bool
		wantJSON bool
		wantErr  bool
	}{
		{"", false, false, false},
		{"csv", true, false, false},
		{"json", false, true, false},
		{"both", true, true, false},
		{"CSV", true, false, false},
		{" Both ", true, true, false},
		{"xml", false, false, true},
		{"csv,json", false, false, true},
	}
	for _, c := range cases {
		gotCSV, gotJSON, err := parseExportFlag(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseExportFlag(%q): expected error", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseExportFlag(%q): %v", c.in, err)
			continue
		}
		if gotCSV != c.wantCSV || gotJSON != c.wantJSON {
			t.Errorf("parseExportFlag(%q) = %v/%v, want %v/%v", c.in, gotCSV, gotJSON, c.wantCSV, c.wantJSON)
		}
	}
}

// TestRunCLIExport checks the whole CLI export contract end to end:
// no files unless -export asks, and only the requested formats.
// The target is a closed localhost port, so it answers Refused
// immediately with no network involved.
func TestRunCLIExport(t *testing.T) {
	cfg := &config.File{Categories: []config.Category{
		{Name: "t", Description: "test", Targets: []config.Target{
			{Host: "127.0.0.1", Port: 1, Method: "tcp"},
		}},
	}}
	o := engine.DefaultOptions()
	o.Packets = 2

	cases := []struct {
		name     string
		wantCSV  bool
		wantJSON bool
		wantDir  bool
	}{
		{"no export by default", false, false, false},
		{"csv only", true, false, true},
		{"json only", false, true, true},
		{"both", true, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			outDir := filepath.Join(t.TempDir(), "out")
			runCLI(context.Background(), cfg, o, "t", outDir, true, engine.NetworkContext{}, c.wantCSV, c.wantJSON)
			if _, err := os.Stat(outDir); os.IsNotExist(err) {
				if c.wantDir {
					t.Fatalf("output dir was not created")
				}
				return
			}
			if !c.wantDir {
				t.Fatalf("output dir was created, but nothing was requested")
			}
			entries, err := os.ReadDir(outDir)
			if err != nil {
				t.Fatalf("read output dir: %v", err)
			}
			var csv, json int
			for _, e := range entries {
				switch {
				case strings.HasSuffix(e.Name(), ".csv"):
					csv++
				case strings.HasSuffix(e.Name(), ".json"):
					json++
				default:
					t.Errorf("unexpected file %q in output dir", e.Name())
				}
			}
			if csv != boolToInt(c.wantCSV) || json != boolToInt(c.wantJSON) {
				t.Fatalf("files: csv=%d json=%d, want csv=%v json=%v", csv, json, c.wantCSV, c.wantJSON)
			}
		})
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
