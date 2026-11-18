// Package report renders test results: the ranking table, the
// ASCII latency chart, the per-target detail view, and the CSV and
// JSON exports. Colours are standard ANSI only and are used only
// when ColorEnabled is set (terminal output); quiet and piped
// output stays plain.
package report

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/erffanhub-00/net-tester/internal/engine"
)

// ColorEnabled controls ANSI colour output. The main program sets
// it when stdout is a terminal and NO_COLOR is unset.
var ColorEnabled bool

const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
)

func color(code, s string) string {
	if !ColorEnabled {
		return s
	}
	return code + s + colorReset
}

func statusColor(status string) string {
	switch status {
	case engine.StatusOK:
		return color(colorGreen, status)
	case engine.StatusPartial:
		return color(colorYellow, status)
	default:
		return color(colorRed, status)
	}
}

// Table renders the ranking table.
func Table(w io.Writer, results []*engine.Result) {
	fmt.Fprintln(w, "Rank  Target                 Status       Loss    Avg     P95     Jitter  Score  Grade")
	fmt.Fprintln(w, "----  ---------------------  -----------  ------  ------  ------  ------  -----  ---------")
	for _, r := range results {
		name := displayName(r)
		rank := "-"
		if r.Rank > 0 {
			rank = strconv.Itoa(r.Rank)
		}
		fmt.Fprintf(w, "%-4s  %-21s  %-11s  %5.1f%%  %6.1f  %6.1f  %6.1f  %5.1f  %s\n",
			rank, truncate(name, 21), statusColor(r.DisplayStatus()),
			r.LossPct, r.Avg, r.P95, r.Jitter, r.Score, r.Grade)
	}
}

// Chart renders an ASCII latency chart (average bars, longest first).
func Chart(w io.Writer, results []*engine.Result) {
	rows := make([]*engine.Result, 0, len(results))
	for _, r := range results {
		if !r.Failed() {
			rows = append(rows, r)
		}
	}
	if len(rows) == 0 {
		return
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Avg > rows[j].Avg })
	maxAvg := rows[0].Avg
	if maxAvg <= 0 {
		maxAvg = 1
	}
	fmt.Fprintln(w, "\nLatency (avg ms):")
	for _, r := range rows {
		name := displayName(r)
		bars := int(r.Avg / maxAvg * 40)
		if bars == 0 && r.Avg > 0 {
			bars = 1
		}
		fmt.Fprintf(w, "  %-22s |%-40s| %6.1f ms\n", truncate(name, 22), strings.Repeat("#", bars), r.Avg)
	}
}

// Summary prints the totals block after a run.
func Summary(w io.Writer, results []*engine.Result) {
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
	fmt.Fprintf(w, "\n%d targets tested, %d OK, %d failed.\n", total, total-failed, failed)
	if best != nil {
		fmt.Fprintf(w, "Top ranked: %s (score %.1f, %s)\n", best.Target.Host, best.Score, best.Grade)
	}
}

// Detail renders the per-target detail view.
func Detail(w io.Writer, r *engine.Result) {
	fmt.Fprintf(w, "\n%s\n", color(colorBold, "Detail: "+r.Target.Host))
	fmt.Fprintf(w, "  Method:   %s\n", r.Target.Method)
	if r.Target.Port != 0 {
		fmt.Fprintf(w, "  Port:     %d\n", r.Target.Port)
	}
	if r.ResolvedIP != "" {
		fmt.Fprintf(w, "  Resolved: %s\n", r.ResolvedIP)
	}
	fmt.Fprintf(w, "  Status:   %s\n", statusColor(r.DisplayStatus()))
	fmt.Fprintf(w, "  Samples:  %d sent, %d received, %.1f%% loss\n", r.Sent, r.Received, r.LossPct)
	if !r.Failed() {
		fmt.Fprintf(w, "  Latency:  avg %.1f ms, min %.1f, max %.1f, p50 %.1f, p95 %.1f, p99 %.1f\n",
			r.Avg, r.Min, r.Max, r.P50, r.P95, r.P99)
		fmt.Fprintf(w, "  Jitter:   %.1f ms, stddev %.1f ms\n", r.Jitter, r.StdDev)
		if r.DNSMs > 0 {
			fmt.Fprintf(w, "  DNS:      %.1f ms\n", r.DNSMs)
		}
		if r.ConnectMs > 0 {
			fmt.Fprintf(w, "  Connect:  %.1f ms\n", r.ConnectMs)
		}
		if r.TTFBMs > 0 {
			fmt.Fprintf(w, "  TTFB:     %.1f ms\n", r.TTFBMs)
		}
		if r.HTTPStatus != 0 {
			fmt.Fprintf(w, "  HTTP:     %d %s\n", r.HTTPStatus, formatDistribution(r.HTTPDistribution))
		}
		if r.TLSValid != nil {
			fmt.Fprintf(w, "  TLS:      valid=%v, handshake %.1f ms\n", *r.TLSValid, r.TLSHandshakeMs)
		}
		fmt.Fprintf(w, "  Score:    %.1f (%s)\n", r.Score, r.Grade)
		// Per-sample latency bars.
		max := r.Max
		if max <= 0 {
			max = 1
		}
		fmt.Fprintln(w, "  Samples:")
		for i, s := range r.Samples {
			bars := int(s / max * 30)
			fmt.Fprintf(w, "    #%d %-30s %6.1f ms\n", i+1, strings.Repeat("#", bars), s)
		}
	}
	if r.Error != "" {
		fmt.Fprintf(w, "  Error:    %s\n", r.Error)
	}
}

func formatDistribution(dist map[int]int) string {
	if len(dist) == 0 {
		return ""
	}
	parts := make([]string, 0, len(dist))
	for code, n := range dist {
		parts = append(parts, fmt.Sprintf("%dx%d", code, n))
	}
	sort.Strings(parts)
	return "(" + strings.Join(parts, ", ") + ")"
}

// Export writes timestamped CSV and JSON files to dir and returns
// the two file paths. The directory is created if needed. The JSON
// carries no network context; use ExportWithNetwork when one was
// detected for the run.
func Export(dir, category string, results []*engine.Result) (csvPath, jsonPath string, err error) {
	return ExportWithNetwork(dir, category, results, engine.NetworkContext{})
}

// networkDoc is the "network" object in the JSON export: what the
// run went through. The raw proxy URL is never written (it may
// contain credentials); only whether a proxy was active and where
// it came from.
type networkDoc struct {
	ProxyActive   bool     `json:"proxy_active"`
	ProxySource   string   `json:"proxy_source,omitempty"`
	VPNInterfaces []string `json:"vpn_interfaces,omitempty"`
	Notes         []string `json:"notes,omitempty"`
}

// ExportSelected writes only the requested formats (timestamped
// files in dir) and returns their paths — "" for a format not
// requested. When both are false it writes nothing and returns
// ("", "", nil) without even creating the directory, so callers can
// honour "no export unless the user asked for it".
func ExportSelected(dir, category string, results []*engine.Result, nc engine.NetworkContext, wantCSV, wantJSON bool) (csvPath, jsonPath string, err error) {
	if !wantCSV && !wantJSON {
		return "", "", nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", fmt.Errorf("cannot create output dir: %w", err)
	}
	stamp := time.Now().Format("20060102-150405")
	base := "net-tester_" + category + "_" + stamp
	if wantCSV {
		csvPath = filepath.Join(dir, base+".csv")
		if err := WriteCSV(csvPath, results); err != nil {
			return "", "", err
		}
	}
	if wantJSON {
		jsonPath = filepath.Join(dir, base+".json")
		if err := WriteJSON(jsonPath, category, results, nc); err != nil {
			return "", "", err
		}
	}
	return csvPath, jsonPath, nil
}

// ExportWithNetwork is Export, plus the detected network context
// (proxy / VPN interfaces) and its run-level notes in the JSON.
func ExportWithNetwork(dir, category string, results []*engine.Result, nc engine.NetworkContext) (csvPath, jsonPath string, err error) {
	return ExportSelected(dir, category, results, nc, true, true)
}

var csvHeader = []string{
	"target", "host", "port", "method", "status", "score", "grade",
	"avg_ms", "min_ms", "max_ms", "p50_ms", "p95_ms", "p99_ms",
	"jitter_ms", "stddev_ms", "loss_pct", "sent", "received",
	"resolved_ip", "dns_ms", "connect_ms", "ttfb_ms", "total_ms",
	"http_status", "tls_valid",
}

// WriteCSV writes the results table to path. Export uses it for
// the timestamped file; the GUI uses it for the export dialog.
func WriteCSV(path string, results []*engine.Result) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("cannot create CSV: %w", err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	if err := w.Write(csvHeader); err != nil {
		return err
	}
	for _, r := range results {
		tlsValid := ""
		if r.TLSValid != nil {
			tlsValid = strconv.FormatBool(*r.TLSValid)
		}
		row := []string{
			r.Target.Key(), r.Target.Host, strconv.Itoa(r.Target.Port), r.Target.Method,
			r.DisplayStatus(), fmt.Sprintf("%.1f", r.Score), r.Grade,
			f1(r.Avg), f1(r.Min), f1(r.Max), f1(r.P50), f1(r.P95), f1(r.P99),
			f1(r.Jitter), f1(r.StdDev), f1(r.LossPct),
			strconv.Itoa(r.Sent), strconv.Itoa(r.Received),
			r.ResolvedIP, f1(r.DNSMs), f1(r.ConnectMs), f1(r.TTFBMs), f1(r.TotalMs),
			itoaOrEmpty(r.HTTPStatus), tlsValid,
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

type jsonDoc struct {
	Category  string           `json:"category"`
	Generated string           `json:"generated_at"`
	Network   *networkDoc      `json:"network,omitempty"`
	Results   []*engine.Result `json:"results"`
}

// WriteJSON writes the results document to path. Export uses it
// for the timestamped file; the GUI uses it for the export dialog.
// nc is included as the top-level "network" object; pass an empty
// NetworkContext to omit it.
func WriteJSON(path, category string, results []*engine.Result, nc engine.NetworkContext) error {
	doc := jsonDoc{
		Category:  category,
		Generated: time.Now().UTC().Format(time.RFC3339),
		Results:   results,
	}
	if nc.ProxyActive() || len(nc.VPNInterfaces) > 0 {
		doc.Network = &networkDoc{
			ProxyActive:   nc.ProxyActive(),
			ProxySource:   nc.ProxySource(),
			VPNInterfaces: nc.VPNInterfaces,
			Notes:         nc.Notes(),
		}
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func f1(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }

func itoaOrEmpty(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n)
}

// displayName renders a target as host:port/method (port omitted
// when it does not apply, e.g. icmp/dns), so two targets on the
// same host stay distinguishable.
func displayName(r *engine.Result) string {
	name := r.Target.Host
	if r.Target.Port != 0 {
		name += ":" + strconv.Itoa(r.Target.Port)
	}
	return name + "/" + r.Target.Method
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "~"
}
