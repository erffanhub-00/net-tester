// Package menu implements the interactive mode: a plain
// line-based menu that works on every terminal, including plain
// cmd.exe on Windows. There is deliberately no TUI library here:
// the menu is prompts and typed answers, so it cannot break on a
// terminal that lacks ANSI support.
package menu

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/erffanhub-00/net-tester/internal/config"
	"github.com/erffanhub-00/net-tester/internal/engine"
	"github.com/erffanhub-00/net-tester/internal/report"
)

// Runner runs the tests and exports the results. It is injected so
// the menu can be tested without the network.
type Runner func(ctx context.Context, targets []config.Target, o engine.Options) []*engine.Result

// Exporter writes the CSV and JSON files.
type Exporter func(dir, category string, results []*engine.Result) (string, string, error)

// Run is the interactive loop. It returns the process exit code:
// 0 when every tested target was OK, 1 when any target failed.
func Run(ctx context.Context, cfg *config.File, o engine.Options, outDir string, in io.Reader, out io.Writer, run Runner, export Exporter) int {
	rd := bufio.NewReader(in)
	lastExit := 0
	for {
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "net-tester — pick a category to test")
		for i, c := range cfg.Categories {
			fmt.Fprintf(out, "  %d. %s — %s (%d targets)\n", i+1, c.Name, c.Description, len(c.Targets))
		}
		fmt.Fprintln(out, "  0. Exit")
		fmt.Fprint(out, "Category (number, 1,3,5 or 1-3, A for all, 0 to exit): ")
		line, _ := rd.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "0" || strings.EqualFold(line, "exit") || strings.EqualFold(line, "q") {
			return lastExit
		}
		cats, err := pickCategories(cfg, line)
		if err != nil {
			fmt.Fprintf(out, "Invalid choice: %v\n", err)
			continue
		}
		packets := askPackets(rd, out, o.Packets)
		oo := o
		oo.Packets = packets
		var targets []config.Target
		var catName string
		for _, c := range cats {
			targets = append(targets, c.Targets...)
			if catName == "" {
				catName = c.Name
			} else {
				catName += "+" + c.Name
			}
		}
		fmt.Fprintf(out, "\nTesting %d targets (%s), %d samples each...\n", len(targets), catName, packets)
		results := run(ctx, targets, oo)
		report.Table(out, results)
		report.Chart(out, results)
		report.Summary(out, results)
		csvPath, jsonPath, err := export(outDir, catName, results)
		if err != nil {
			fmt.Fprintf(out, "Export failed: %v\n", err)
			lastExit = 1
		} else {
			fmt.Fprintf(out, "Saved: %s\n       %s\n", csvPath, jsonPath)
		}
		if engine.AnyFailed(results) {
			lastExit = 1
		}
		detailLoop(rd, out, results)
	}
}

// pickCategories parses the category picker input.
func pickCategories(cfg *config.File, input string) ([]config.Category, error) {
	input = strings.TrimSpace(input)
	if strings.EqualFold(input, "a") || strings.EqualFold(input, "all") {
		return cfg.Categories, nil
	}
	var idxs []int
	for _, part := range strings.Split(input, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "-") {
			bounds := strings.SplitN(part, "-", 2)
			lo, err1 := strconv.Atoi(strings.TrimSpace(bounds[0]))
			hi, err2 := strconv.Atoi(strings.TrimSpace(bounds[1]))
			if err1 != nil || err2 != nil || lo < 1 || hi > len(cfg.Categories) || lo > hi {
				return nil, fmt.Errorf("bad range %q (categories are 1-%d)", part, len(cfg.Categories))
			}
			for i := lo; i <= hi; i++ {
				idxs = append(idxs, i)
			}
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 1 || n > len(cfg.Categories) {
			return nil, fmt.Errorf("bad choice %q (categories are 1-%d)", part, len(cfg.Categories))
		}
		idxs = append(idxs, n)
	}
	if len(idxs) == 0 {
		return nil, fmt.Errorf("nothing selected")
	}
	seen := map[int]bool{}
	out := make([]config.Category, 0, len(idxs))
	for _, i := range idxs {
		if !seen[i] {
			seen[i] = true
			out = append(out, cfg.Categories[i-1])
		}
	}
	return out, nil
}

// askPackets prompts for the sample count, defaulting to def.
func askPackets(rd *bufio.Reader, out io.Writer, def int) int {
	fmt.Fprintf(out, "Samples per target [%d, 1-200]: ", def)
	line, _ := rd.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return def
	}
	n, err := strconv.Atoi(line)
	if err != nil || n < 1 || n > 200 {
		fmt.Fprintf(out, "Invalid number, using %d.\n", def)
		return def
	}
	return n
}

// detailLoop offers the per-target detail view after a run.
func detailLoop(rd *bufio.Reader, out io.Writer, results []*engine.Result) {
	for {
		fmt.Fprint(out, "Details for a target (rank number, Enter to continue): ")
		line, _ := rd.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			return
		}
		n, err := strconv.Atoi(line)
		if err != nil || n < 1 {
			fmt.Fprintln(out, "Enter a rank number, or just Enter to continue.")
			continue
		}
		var found *engine.Result
		for _, r := range results {
			if r.Rank == n {
				found = r
				break
			}
		}
		if found == nil {
			// Unranked (failed) targets have no rank; match by row.
			if n <= len(results) {
				found = results[n-1]
			}
		}
		if found == nil {
			fmt.Fprintln(out, "No target with that number.")
			continue
		}
		report.Detail(out, found)
	}
}
