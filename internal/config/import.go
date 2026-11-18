// Bulk import: add many targets at once from pasted text or a file.
//
// Line format (one target per line; blank lines and lines starting
// with '#' are ignored):
//
//	host
//	host:port
//	host method
//	host:port method
//
// Method defaults: no port → icmp; port given → tcp. Port defaults
// by method: icmp/dns → 0, http → 80, https → 443, tcp → 443.
// A .json file is a categories.json file and is merged instead
// (see MergeFile).
package config

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// defaultPort is the port a method gets when the line gives none.
func defaultPort(method string) int {
	switch method {
	case MethodHTTP:
		return 80
	case MethodHTTPS, MethodTCP:
		return 443
	default: // icmp, dns
		return 0
	}
}

// ParseTargetLine parses one line of the bulk-import format. It
// returns skip=true for blank lines and comments (no target, no
// error); otherwise t holds the target or err explains why the line
// is unusable.
func ParseTargetLine(line string) (t Target, skip bool, err error) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return Target{}, true, nil
	}
	fields := strings.Fields(trimmed)
	if len(fields) > 2 {
		return Target{}, false, fmt.Errorf("too many fields (want: host[:port] [method])")
	}
	host, port, portGiven, err := splitHostPort(fields[0])
	if err != nil {
		return Target{}, false, err
	}
	method := ""
	if len(fields) == 2 {
		method = strings.ToLower(strings.TrimSpace(fields[1]))
		switch method {
		case MethodICMP, MethodTCP, MethodHTTP, MethodHTTPS, MethodDNS:
		default:
			return Target{}, false, fmt.Errorf("unknown method %q (want icmp, tcp, http, https or dns)", fields[1])
		}
	}
	if method == "" {
		if portGiven {
			method = MethodTCP
		} else {
			method = MethodICMP
		}
	}
	if !portGiven {
		port = defaultPort(method)
	}
	t = Target{Host: host, Port: port, Method: method}
	if err := ValidateTarget(&t); err != nil {
		return Target{}, false, err
	}
	return t, false, nil
}

// splitHostPort splits a host[:port] field. Bracketed IPv6
// ([::1]:8080) goes through net.SplitHostPort; anything else is
// split at the last colon. A bare host has no port.
func splitHostPort(field string) (host string, port int, portGiven bool, err error) {
	if strings.Contains(field, "://") {
		return "", 0, false, fmt.Errorf("looks like a URL — paste host[:port] [method] instead")
	}
	if strings.HasPrefix(field, "[") {
		h, p, err := net.SplitHostPort(field)
		if err != nil {
			return "", 0, false, fmt.Errorf("bad host:port %q", field)
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return "", 0, false, fmt.Errorf("bad port %q", p)
		}
		return h, n, true, nil
	}
	if i := strings.LastIndex(field, ":"); i >= 0 {
		host, portStr := field[:i], field[i+1:]
		if host == "" || strings.Contains(host, ":") || strings.Contains(host, "/") {
			return "", 0, false, fmt.Errorf("bad host %q", field)
		}
		if portStr == "" {
			return "", 0, false, fmt.Errorf("bad port in %q (empty port)", field)
		}
		n, err := strconv.Atoi(portStr)
		if err != nil {
			return "", 0, false, fmt.Errorf("bad port %q", portStr)
		}
		return host, n, true, nil
	}
	if strings.Contains(field, "/") {
		return "", 0, false, fmt.Errorf("bad host %q", field)
	}
	return field, 0, false, nil
}

// ImportSkip records one pasted line that was not imported.
type ImportSkip struct {
	Line   string
	Reason string
}

// ParseImport parses pasted/file lines into targets, skipping
// duplicates (against existing and within the batch) and bad lines.
// Blank lines and comments are ignored silently. It returns the new
// targets and one entry per skipped line, in input order.
func ParseImport(text string, existing []Target) (added []Target, skipped []ImportSkip) {
	seen := map[string]bool{}
	for _, t := range existing {
		seen[t.Key()] = true
	}
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		t, skip, err := ParseTargetLine(line)
		if skip {
			continue
		}
		if err != nil {
			skipped = append(skipped, ImportSkip{Line: trimmed, Reason: err.Error()})
			continue
		}
		if seen[t.Key()] {
			skipped = append(skipped, ImportSkip{Line: trimmed, Reason: "duplicate target"})
			continue
		}
		seen[t.Key()] = true
		added = append(added, t)
	}
	return added, skipped
}

// MergeFile merges a parsed categories file into dst: categories
// that do not exist yet are appended whole; for categories that
// already exist, only the non-duplicate targets are appended. It
// returns the numbers of categories added, targets added and
// targets skipped as duplicates.
func MergeFile(dst, src *File) (addedCategories, addedTargets, skippedTargets int) {
	for _, sc := range src.Categories {
		dc, ok := dst.Find(sc.Name)
		if !ok {
			dst.Categories = append(dst.Categories, sc)
			addedCategories++
			addedTargets += len(sc.Targets)
			continue
		}
		seen := map[string]bool{}
		for _, t := range dc.Targets {
			seen[t.Key()] = true
		}
		for _, t := range sc.Targets {
			if seen[t.Key()] {
				skippedTargets++
				continue
			}
			seen[t.Key()] = true
			dc.Targets = append(dc.Targets, t)
			addedTargets++
		}
	}
	return addedCategories, addedTargets, skippedTargets
}
