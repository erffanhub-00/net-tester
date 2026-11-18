package config

import (
	"strings"
	"testing"
)

const validJSON = `{"categories": [
  {"name": "cdn", "description": "CDN endpoints", "targets": [
    {"host": "1.1.1.1", "port": 443, "method": "https"},
    {"host": "example.com", "port": 0, "method": "icmp"}
  ]},
  {"name": "dns", "description": "Resolvers", "targets": [
    {"host": "8.8.8.8", "port": 0, "method": "dns"}
  ]}
]}`

func TestParseValid(t *testing.T) {
	f, err := Parse([]byte(validJSON))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(f.Categories) != 2 {
		t.Fatalf("got %d categories, want 2", len(f.Categories))
	}
	if got := f.Names(); got[0] != "cdn" || got[1] != "dns" {
		t.Fatalf("Names = %v", got)
	}
	c, ok := f.Find("CDN")
	if !ok || c.Name != "cdn" {
		t.Fatalf("Find(CDN) = %v, %v", c, ok)
	}
}

func TestParseRejectsBadJSON(t *testing.T) {
	if _, err := Parse([]byte(`{oops`)); err == nil {
		t.Fatal("expected error for bad JSON")
	}
}

func TestParseRejectsEmptyCategories(t *testing.T) {
	if _, err := Parse([]byte(`{"categories": []}`)); err == nil {
		t.Fatal("expected error for empty categories")
	}
}

func TestParseRejectsDuplicateCategory(t *testing.T) {
	data := `{"categories": [
		{"name": "a", "targets": [{"host": "x", "port": 0, "method": "icmp"}]},
		{"name": "A", "targets": [{"host": "y", "port": 0, "method": "icmp"}]}
	]}`
	if _, err := Parse([]byte(data)); err == nil || !strings.Contains(err.Error(), "duplicate category") {
		t.Fatalf("expected duplicate category error, got %v", err)
	}
}

func TestParseRejectsDuplicateTarget(t *testing.T) {
	data := `{"categories": [
		{"name": "a", "targets": [
			{"host": "example.com", "port": 0, "method": "icmp"},
			{"host": "EXAMPLE.COM", "port": 0, "method": "ICMP"}
		]}
	]}`
	if _, err := Parse([]byte(data)); err == nil || !strings.Contains(err.Error(), "duplicate target") {
		t.Fatalf("expected duplicate target error, got %v", err)
	}
}

func TestValidateTarget(t *testing.T) {
	cases := []struct {
		t       Target
		wantErr string
	}{
		{Target{Host: "example.com", Method: "icmp"}, ""},
		{Target{Host: "example.com", Method: "ICMP"}, ""}, // normalized
		{Target{Host: "example.com", Port: 443, Method: "https"}, ""},
		{Target{Host: "8.8.8.8", Method: "dns"}, ""},
		{Target{Host: "", Method: "icmp"}, "host is empty"},
		{Target{Host: "-evil", Method: "icmp"}, "starts with '-'"},
		{Target{Host: "x", Method: "stratum"}, "unknown method"},
		{Target{Host: "x", Port: 0, Method: "tcp"}, "needs a port"},
		{Target{Host: "x", Port: 70000, Method: "tcp"}, "needs a port"},
		{Target{Host: "x", Port: 80, Method: "http"}, ""},
	}
	for i, c := range cases {
		err := ValidateTarget(&c.t)
		if c.wantErr == "" && err != nil {
			t.Errorf("case %d: unexpected error: %v", i, err)
		}
		if c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)) {
			t.Errorf("case %d: expected error containing %q, got %v", i, c.wantErr, err)
		}
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load("/nonexistent/categories.json"); err == nil {
		t.Fatal("expected error for missing file")
	}
}
