package menu

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/erffanhub-00/net-tester/internal/config"
	"github.com/erffanhub-00/net-tester/internal/engine"
	"github.com/erffanhub-00/net-tester/internal/report"
)

func testConfig() *config.File {
	return &config.File{Categories: []config.Category{
		{Name: "cdn", Description: "CDN", Targets: []config.Target{{Host: "a.example", Method: "icmp"}}},
		{Name: "dns", Description: "DNS", Targets: []config.Target{{Host: "1.1.1.1", Method: "dns"}}},
		{Name: "cloud", Description: "Cloud", Targets: []config.Target{{Host: "b.example", Port: 443, Method: "https"}}},
	}}
}

func TestPickCategories(t *testing.T) {
	cfg := testConfig()
	cases := []struct {
		in   string
		want []string
	}{
		{"1", []string{"cdn"}},
		{"A", []string{"cdn", "dns", "cloud"}},
		{"all", []string{"cdn", "dns", "cloud"}},
		{"1,3", []string{"cdn", "cloud"}},
		{"1-2", []string{"cdn", "dns"}},
		{"2,2,1", []string{"dns", "cdn"}}, // deduped, order kept
	}
	for _, c := range cases {
		got, err := pickCategories(cfg, c.in)
		if err != nil {
			t.Fatalf("pickCategories(%q): %v", c.in, err)
		}
		names := make([]string, len(got))
		for i, cat := range got {
			names[i] = cat.Name
		}
		if strings.Join(names, ",") != strings.Join(c.want, ",") {
			t.Errorf("pickCategories(%q) = %v, want %v", c.in, names, c.want)
		}
	}
	for _, bad := range []string{"", "0", "4", "x", "2-1", "1-9"} {
		if _, err := pickCategories(cfg, bad); err == nil {
			t.Errorf("pickCategories(%q): expected error", bad)
		}
	}
}

func fakeRunner(results []*engine.Result) Runner {
	return func(ctx context.Context, targets []config.Target, o engine.Options) []*engine.Result {
		return results
	}
}

func fakeExporter(t *testing.T) Exporter {
	t.Helper()
	return func(dir, category string, results []*engine.Result) (string, string, error) {
		return dir + "/x.csv", dir + "/x.json", nil
	}
}

func TestMenuRunAndExit(t *testing.T) {
	report.ColorEnabled = false
	in := bytes.NewBufferString("1\n\n\n0\n")
	var out bytes.Buffer
	res := []*engine.Result{{
		Target:  config.Target{Host: "a.example", Method: "icmp"},
		Status:  engine.StatusOK,
		Samples: []float64{10, 11},
		Sent:    2, Received: 2,
		Avg: 10.5, Score: 95, Grade: "Excellent", Rank: 1,
	}}
	code := Run(context.Background(), testConfig(), engine.DefaultOptions(), "output", in, &out, fakeRunner(res), fakeExporter(t))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	text := out.String()
	for _, want := range []string{"cdn", "a.example", "Excellent", "Saved:"} {
		if !strings.Contains(text, want) {
			t.Fatalf("menu output missing %q:\n%s", want, text)
		}
	}
}

func TestMenuExitCodeOnFailure(t *testing.T) {
	report.ColorEnabled = false
	in := bytes.NewBufferString("2\n5\n\n0\n")
	var out bytes.Buffer
	res := []*engine.Result{{
		Target: config.Target{Host: "1.1.1.1", Method: "dns"},
		Status: engine.StatusTimeout,
		Sent:   5, Received: 0, LossPct: 100,
		Score: 0, Grade: "Failed", Rank: 0,
	}}
	code := Run(context.Background(), testConfig(), engine.DefaultOptions(), "output", in, &out, fakeRunner(res), fakeExporter(t))
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (a target failed)", code)
	}
}

// TestMenuNoExportPrintsNothing checks that when the exporter wrote
// no files (no -export given), the menu prints no "Saved:" line.
func TestMenuNoExportPrintsNothing(t *testing.T) {
	report.ColorEnabled = false
	in := bytes.NewBufferString("1\n\n\n0\n")
	var out bytes.Buffer
	res := []*engine.Result{{
		Target:  config.Target{Host: "a.example", Method: "icmp"},
		Status:  engine.StatusOK,
		Samples: []float64{10, 11},
		Sent:    2, Received: 2,
		Avg: 10.5, Score: 95, Grade: "Excellent", Rank: 1,
	}}
	noExport := func(dir, category string, results []*engine.Result) (string, string, error) {
		return "", "", nil
	}
	code := Run(context.Background(), testConfig(), engine.DefaultOptions(), "output", in, &out, fakeRunner(res), noExport)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	text := out.String()
	if strings.Contains(text, "Saved:") {
		t.Fatalf("menu printed a Saved: line, but nothing was exported:\n%s", text)
	}
	if !strings.Contains(text, "a.example") {
		t.Fatalf("menu output missing the result row:\n%s", text)
	}
}
