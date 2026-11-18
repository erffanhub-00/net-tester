package report

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erffanhub-00/net-tester/internal/config"
	"github.com/erffanhub-00/net-tester/internal/engine"
)

func sampleResults() []*engine.Result {
	tlsOK := true
	return []*engine.Result{
		{
			Target:  config.Target{Host: "good.example", Port: 443, Method: "https"},
			Status:  engine.StatusOK,
			Samples: []float64{10, 12, 11},
			Sent:    3, Received: 3, LossPct: 0,
			Avg: 11, Min: 10, Max: 12, Jitter: 1, StdDev: 0.8, P50: 11, P95: 11.9, P99: 12,
			ResolvedIP: "93.184.216.34", DNSMs: 5, ConnectMs: 8, TTFBMs: 10, TotalMs: 11,
			HTTPStatus: 200, HTTPDistribution: map[int]int{200: 3},
			TLSValid: &tlsOK, TLSHandshakeMs: 6,
			Score: 98.7, Grade: "Excellent", Rank: 1,
		},
		{
			Target: config.Target{Host: "down.example", Port: 0, Method: "icmp"},
			Status: engine.StatusTimeout,
			Sent:   3, Received: 0, LossPct: 100,
			Score: 0, Grade: "Failed", Rank: 0, Error: "request timed out",
		},
	}
}

func TestTableAndChart(t *testing.T) {
	ColorEnabled = false
	var buf bytes.Buffer
	Table(&buf, sampleResults())
	out := buf.String()
	if !strings.Contains(out, "good.example") || !strings.Contains(out, "98.7") || !strings.Contains(out, "Excellent") {
		t.Fatalf("table missing expected content:\n%s", out)
	}
	if !strings.Contains(out, "down.example") || !strings.Contains(out, "Failed") {
		t.Fatalf("table missing failed row:\n%s", out)
	}
	buf.Reset()
	Chart(&buf, sampleResults())
	if !strings.Contains(buf.String(), "good.example") || !strings.Contains(buf.String(), "#") {
		t.Fatalf("chart missing bars:\n%s", buf.String())
	}
	// Failed targets get no chart row.
	if strings.Contains(buf.String(), "down.example") {
		t.Fatalf("chart should skip failed targets:\n%s", buf.String())
	}
	buf.Reset()
	Detail(&buf, sampleResults()[0])
	d := buf.String()
	for _, want := range []string{"good.example", "93.184.216.34", "TLS", "Score", "Samples:"} {
		if !strings.Contains(d, want) {
			t.Fatalf("detail missing %q:\n%s", want, d)
		}
	}
}

func TestExportRoundTrip(t *testing.T) {
	dir := t.TempDir()
	csvPath, jsonPath, err := Export(dir, "cdn", sampleResults())
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if filepath.Dir(csvPath) != dir || !strings.HasSuffix(csvPath, ".csv") {
		t.Fatalf("csv path = %q", csvPath)
	}
	if !strings.HasSuffix(jsonPath, ".json") {
		t.Fatalf("json path = %q", jsonPath)
	}
	// CSV parses and has header + 2 rows.
	f, err := os.Open(csvPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatalf("csv parse: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("csv rows = %d, want 3 (header + 2)", len(rows))
	}
	if rows[0][0] != "target" || rows[1][1] != "good.example" || rows[1][5] != "98.7" {
		t.Fatalf("csv content wrong: %v", rows)
	}
	// JSON parses and round-trips the results.
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc jsonDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if doc.Category != "cdn" || len(doc.Results) != 2 {
		t.Fatalf("json doc = %+v", doc)
	}
	if doc.Results[0].HTTPStatus != 200 || doc.Results[0].Score != 98.7 {
		t.Fatalf("json result 0 = %+v", doc.Results[0])
	}
}

func TestExportCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "output")
	if _, _, err := Export(dir, "custom", sampleResults()); err != nil {
		t.Fatalf("Export should create the dir: %v", err)
	}
}

func TestExportWithNetwork(t *testing.T) {
	dir := t.TempDir()
	nc := engine.NetworkContext{
		ProxyEnv:      "http://proxy.example:8080",
		VPNInterfaces: []string{"tun0"},
	}
	// ProxySource needs the env var name; same package, so the
	// unexported field can be set directly in the test.
	_, jsonPath, err := ExportWithNetwork(dir, "cdn", sampleResults(), nc)
	if err != nil {
		t.Fatalf("ExportWithNetwork: %v", err)
	}
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Network *struct {
			ProxyActive   bool     `json:"proxy_active"`
			ProxySource   string   `json:"proxy_source"`
			VPNInterfaces []string `json:"vpn_interfaces"`
			Notes         []string `json:"notes"`
		} `json:"network"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if doc.Network == nil {
		t.Fatal("json has no network object")
	}
	if !doc.Network.ProxyActive || len(doc.Network.VPNInterfaces) != 1 || doc.Network.VPNInterfaces[0] != "tun0" {
		t.Fatalf("network = %+v", doc.Network)
	}
	if len(doc.Network.Notes) != 2 {
		t.Fatalf("notes = %v, want VPN + proxy notes", doc.Network.Notes)
	}

	// Plain Export omits the network object entirely.
	_, jsonPath2, err := Export(dir, "cdn", sampleResults())
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	data, err = os.ReadFile(jsonPath2)
	if err != nil {
		t.Fatal(err)
	}
	var doc2 jsonDoc
	if err := json.Unmarshal(data, &doc2); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if doc2.Network != nil {
		t.Fatalf("plain Export should omit network, got %+v", doc2.Network)
	}
}
