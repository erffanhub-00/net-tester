// Package config loads and validates the categories file.
//
// Config model (simplified, by design):
//
//	{"categories": [{"name": "cdn", "description": "...",
//	  "targets": [{"host": "1.1.1.1", "port": 443, "method": "https"}]}]}
//
// A target is one host + one port + one method. There are no
// per-protocol port lists: a category simply groups targets, and a
// category may mix methods in a single run. Port is only required
// for tcp/http/https; icmp and dns targets use port 0.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Valid methods for a target.
const (
	MethodICMP  = "icmp"
	MethodTCP   = "tcp"
	MethodHTTP  = "http"
	MethodHTTPS = "https"
	MethodDNS   = "dns"
)

// Target is a single host to test.
type Target struct {
	Host   string `json:"host"`
	Port   int    `json:"port"`
	Method string `json:"method"`
}

// Key identifies a target for duplicate detection.
func (t Target) Key() string {
	return strings.ToLower(t.Method) + "://" + strings.ToLower(t.Host) + ":" + itoa(t.Port)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// Category is a named group of targets.
type Category struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Targets     []Target `json:"targets"`
}

// File is the on-disk categories file.
type File struct {
	Categories []Category `json:"categories"`
}

// Load reads and validates a categories file from disk.
func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read config %q: %w", path, err)
	}
	return Parse(data)
}

// Parse validates raw JSON config bytes.
func Parse(data []byte) (*File, error) {
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("invalid config JSON: %w", err)
	}
	if len(f.Categories) == 0 {
		return nil, fmt.Errorf("config has no categories")
	}
	seenNames := map[string]bool{}
	for ci := range f.Categories {
		c := &f.Categories[ci]
		if strings.TrimSpace(c.Name) == "" {
			return nil, fmt.Errorf("category %d has no name", ci+1)
		}
		name := strings.ToLower(strings.TrimSpace(c.Name))
		if seenNames[name] {
			return nil, fmt.Errorf("duplicate category name %q", c.Name)
		}
		seenNames[name] = true
		if len(c.Targets) == 0 {
			return nil, fmt.Errorf("category %q has no targets", c.Name)
		}
		seenTargets := map[string]bool{}
		for ti := range c.Targets {
			t := &c.Targets[ti]
			if err := ValidateTarget(t); err != nil {
				return nil, fmt.Errorf("category %q target %d: %w", c.Name, ti+1, err)
			}
			k := t.Key()
			if seenTargets[k] {
				return nil, fmt.Errorf("category %q has duplicate target %s", c.Name, k)
			}
			seenTargets[k] = true
		}
	}
	return &f, nil
}

// ValidateTarget checks one target. Method is normalized to lowercase.
func ValidateTarget(t *Target) error {
	t.Host = strings.TrimSpace(t.Host)
	if t.Host == "" {
		return fmt.Errorf("host is empty")
	}
	if strings.HasPrefix(t.Host, "-") {
		return fmt.Errorf("host %q starts with '-'", t.Host)
	}
	t.Method = strings.ToLower(strings.TrimSpace(t.Method))
	switch t.Method {
	case MethodICMP, MethodTCP, MethodHTTP, MethodHTTPS, MethodDNS:
	default:
		return fmt.Errorf("unknown method %q (want icmp, tcp, http, https or dns)", t.Method)
	}
	switch t.Method {
	case MethodTCP, MethodHTTP, MethodHTTPS:
		if t.Port < 1 || t.Port > 65535 {
			return fmt.Errorf("method %q needs a port between 1 and 65535, got %d", t.Method, t.Port)
		}
	case MethodICMP, MethodDNS:
		if t.Port < 0 || t.Port > 65535 {
			return fmt.Errorf("port out of range: %d", t.Port)
		}
	}
	return nil
}

// Find returns the category with the given name (case-insensitive).
func (f *File) Find(name string) (*Category, bool) {
	for i := range f.Categories {
		if strings.EqualFold(f.Categories[i].Name, name) {
			return &f.Categories[i], true
		}
	}
	return nil, false
}

// Names returns the category names in order.
func (f *File) Names() []string {
	out := make([]string, len(f.Categories))
	for i, c := range f.Categories {
		out[i] = c.Name
	}
	return out
}
