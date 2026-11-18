// Command net-tester is a network tester: ICMP ping, TCP connect,
// HTTP/HTTPS and real DNS resolution checks, with categories,
// scoring and ranking, and CSV + JSON export.
//
// It is the merged rebuild of Network-Ping-Pro (Python) and
// server-tester (PowerShell): one Go binary, no runtime to
// install, same behaviour on Windows, Linux and macOS.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/erffanhub-00/net-tester/internal/config"
	"github.com/erffanhub-00/net-tester/internal/engine"
	"github.com/erffanhub-00/net-tester/internal/menu"
	"github.com/erffanhub-00/net-tester/internal/report"
)

const version = "2.0.0"

func main() {
	setupConsole()

	categoryFlag := flag.String("category", "", "Category to test (comma-separated names, or \"all\"); required with -no-menu")
	packetsFlag := flag.Int("packets", 10, "Samples per target (1-200)")
	configFlag := flag.String("config", "", "Categories file (default: ./categories.json, else the built-in seed)")
	outputFlag := flag.String("output-dir", "output", "Directory for CSV and JSON exports")
	noMenuFlag := flag.Bool("no-menu", false, "Run without the interactive menu (requires -category)")
	workersFlag := flag.Int("workers", 10, "Parallel targets (1-50)")
	pingTimeoutFlag := flag.Int("ping-timeout-ms", 2000, "Per-reply ping timeout in ms (100-60000)")
	tcpTimeoutFlag := flag.Int("tcp-timeout-ms", 3000, "TCP connect timeout in ms (100-60000)")
	httpTimeoutFlag := flag.Int("http-timeout-ms", 5000, "HTTP request timeout in ms (100-60000)")
	dnsTimeoutFlag := flag.Int("dns-timeout-ms", 3000, "DNS lookup timeout in ms (100-60000)")
	proxyFlag := flag.String("proxy", "", "HTTP proxy URL (HTTP only)")
	quietFlag := flag.Bool("quiet", false, "One line per target, no table or chart")
	versionFlag := flag.Bool("version", false, "Print the version and exit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "net-tester %s — network tester (ping, TCP, HTTP/HTTPS, DNS)\n\n", version)
		fmt.Fprintf(os.Stderr, "Usage:\n  net-tester                  interactive menu\n  net-tester -no-menu -category cdn\n  net-tester -no-menu -category cdn,dns -packets 20\n\nFlags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExit codes: 0 all targets OK, 1 a target failed, 2 usage error.\n")
	}
	flag.Parse()

	if *versionFlag {
		fmt.Printf("net-tester %s\n", version)
		os.Exit(0)
	}

	// Validate numeric flags before doing anything else.
	opts, err := buildOptions(*packetsFlag, *workersFlag, *pingTimeoutFlag, *tcpTimeoutFlag, *httpTimeoutFlag, *dnsTimeoutFlag, *proxyFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		flag.Usage()
		os.Exit(2)
	}

	cfg, err := loadConfig(*configFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}

	// Network context (proxy / VPN): detected once, noted once per
	// run on stderr so scripted stdout stays clean.
	nc := engine.DetectNetworkContext(*proxyFlag)
	for _, note := range nc.Notes() {
		fmt.Fprintf(os.Stderr, "note: %s\n", note)
	}

	report.ColorEnabled = useColor(*quietFlag)

	ctx := context.Background()
	if *noMenuFlag {
		os.Exit(runCLI(ctx, cfg, opts, *categoryFlag, *outputFlag, *quietFlag, nc))
	}
	if *categoryFlag != "" {
		fmt.Fprintln(os.Stderr, "note: -category is only used with -no-menu; starting the interactive menu")
	}
	exporter := func(dir, cat string, res []*engine.Result) (string, string, error) {
		return report.ExportWithNetwork(dir, cat, res, nc)
	}
	os.Exit(menu.Run(ctx, cfg, opts, *outputFlag, os.Stdin, os.Stdout,
		engine.RunTargets, exporter))
}

func buildOptions(packets, workers, pingMs, tcpMs, httpMs, dnsMs int, proxy string) (engine.Options, error) {
	o := engine.DefaultOptions()
	if packets < 1 || packets > 200 {
		return o, fmt.Errorf("-packets must be 1-200, got %d", packets)
	}
	if workers < 1 || workers > 50 {
		return o, fmt.Errorf("-workers must be 1-50, got %d", workers)
	}
	for name, ms := range map[string]int{
		"ping-timeout-ms": pingMs, "tcp-timeout-ms": tcpMs,
		"http-timeout-ms": httpMs, "dns-timeout-ms": dnsMs,
	} {
		if ms < 100 || ms > 60000 {
			return o, fmt.Errorf("-%s must be 100-60000, got %d", name, ms)
		}
	}
	o.Packets = packets
	o.Workers = workers
	o.PingTimeout = time.Duration(pingMs) * time.Millisecond
	o.TCPTimeout = time.Duration(tcpMs) * time.Millisecond
	o.HTTPTimeout = time.Duration(httpMs) * time.Millisecond
	o.DNSTimeout = time.Duration(dnsMs) * time.Millisecond
	o.Proxy = proxy
	return o, nil
}

// loadConfig reads the categories file: the -config flag first,
// then ./categories.json next to the working directory, then the
// built-in seed embedded in the binary.
func loadConfig(flagPath string) (*config.File, error) {
	if flagPath != "" {
		return config.Load(flagPath)
	}
	if _, err := os.Stat("categories.json"); err == nil {
		return config.Load("categories.json")
	}
	return config.Seed()
}

// runCLI is the non-interactive mode: -no-menu -category <names>.
func runCLI(ctx context.Context, cfg *config.File, o engine.Options, categoryFlag, outDir string, quiet bool, nc engine.NetworkContext) int {
	if strings.TrimSpace(categoryFlag) == "" {
		fmt.Fprintln(os.Stderr, "error: -no-menu requires -category (e.g. -category cdn or -category all)")
		return 2
	}
	var targets []config.Target
	var catName string
	if strings.EqualFold(strings.TrimSpace(categoryFlag), "all") {
		for _, c := range cfg.Categories {
			targets = append(targets, c.Targets...)
		}
		catName = "all"
	} else {
		for _, name := range strings.Split(categoryFlag, ",") {
			name = strings.TrimSpace(name)
			c, ok := cfg.Find(name)
			if !ok {
				fmt.Fprintf(os.Stderr, "error: unknown category %q (available: %s)\n", name, strings.Join(cfg.Names(), ", "))
				return 2
			}
			targets = append(targets, c.Targets...)
			if catName == "" {
				catName = c.Name
			} else {
				catName += "+" + c.Name
			}
		}
	}
	results := engine.RunTargets(ctx, targets, o)
	if quiet {
		for _, r := range results {
			fmt.Printf("%s %s %.1f %s\n", r.Target.Host, r.DisplayStatus(), r.Score, r.Grade)
		}
	} else {
		report.Table(os.Stdout, results)
		report.Chart(os.Stdout, results)
		report.Summary(os.Stdout, results)
	}
	csvPath, jsonPath, err := report.ExportWithNetwork(outDir, catName, results, nc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "export failed: %v\n", err)
		return 1
	}
	if !quiet {
		fmt.Printf("Saved: %s\n       %s\n", csvPath, jsonPath)
	}
	if engine.AnyFailed(results) {
		return 1
	}
	return 0
}

// useColor decides whether ANSI colours are safe: only when stdout
// is a terminal, colours are not disabled, and output is not quiet.
func useColor(quiet bool) bool {
	if quiet || os.Getenv("NO_COLOR") != "" {
		return false
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
