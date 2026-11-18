package config

import (
	"strings"
	"testing"
)

func TestParseTargetLine(t *testing.T) {
	cases := []struct {
		line       string
		want       Target
		wantSkip   bool
		wantErrSub string
	}{
		{"example.com", Target{Host: "example.com", Port: 0, Method: "icmp"}, false, ""},
		{"example.com:443", Target{Host: "example.com", Port: 443, Method: "tcp"}, false, ""},
		{"example.com https", Target{Host: "example.com", Port: 443, Method: "https"}, false, ""},
		{"example.com http", Target{Host: "example.com", Port: 80, Method: "http"}, false, ""},
		{"example.com:8080 http", Target{Host: "example.com", Port: 8080, Method: "http"}, false, ""},
		{"8.8.8.8 dns", Target{Host: "8.8.8.8", Port: 0, Method: "dns"}, false, ""},
		{"8.8.8.8:53 dns", Target{Host: "8.8.8.8", Port: 53, Method: "dns"}, false, ""},
		{"EXAMPLE.COM HTTPS", Target{Host: "EXAMPLE.COM", Port: 443, Method: "https"}, false, ""},
		{"  google.com:443  ", Target{Host: "google.com", Port: 443, Method: "tcp"}, false, ""},
		{"[::1]:8080 tcp", Target{Host: "::1", Port: 8080, Method: "tcp"}, false, ""},
		{"", Target{}, true, ""},
		{"   ", Target{}, true, ""},
		{"# a comment", Target{}, true, ""},
		{"example.com:abc", Target{}, false, "bad port"},
		{"example.com:", Target{}, false, "bad port"},
		{"example.com:99999", Target{}, false, "port"},
		{"example.com ftp", Target{}, false, "unknown method"},
		{"a b c", Target{}, false, "too many fields"},
		{"https://example.com", Target{}, false, "URL"},
		{"example.com:0 tcp", Target{}, false, "port"},
		{"-bad.example", Target{}, false, "starts with '-'"},
	}
	for _, c := range cases {
		got, skip, err := ParseTargetLine(c.line)
		if skip != c.wantSkip {
			t.Errorf("ParseTargetLine(%q): skip = %v, want %v", c.line, skip, c.wantSkip)
			continue
		}
		if c.wantSkip {
			continue
		}
		if c.wantErrSub != "" {
			if err == nil || !strings.Contains(err.Error(), c.wantErrSub) {
				t.Errorf("ParseTargetLine(%q): err = %v, want containing %q", c.line, err, c.wantErrSub)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseTargetLine(%q): unexpected error: %v", c.line, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseTargetLine(%q) = %+v, want %+v", c.line, got, c.want)
		}
	}
}

func TestParseImport(t *testing.T) {
	text := `
# my servers
example.com
example.com:443
google.com https
bad:port
example.com
8.8.8.8 dns
`
	existing := []Target{{Host: "google.com", Port: 443, Method: "https"}}
	added, skipped := ParseImport(text, existing)
	if len(added) != 3 {
		t.Fatalf("added = %d, want 3: %+v", len(added), added)
	}
	want := []Target{
		{Host: "example.com", Port: 0, Method: "icmp"},
		{Host: "example.com", Port: 443, Method: "tcp"},
		{Host: "8.8.8.8", Port: 0, Method: "dns"},
	}
	for i, w := range want {
		if added[i] != w {
			t.Errorf("added[%d] = %+v, want %+v", i, added[i], w)
		}
	}
	if len(skipped) != 3 {
		t.Fatalf("skipped = %d, want 3: %+v", len(skipped), skipped)
	}
	// google.com https is a duplicate of existing; example.com
	// appears twice; bad:port is a bad line.
	reasons := map[string]bool{}
	for _, s := range skipped {
		reasons[s.Reason] = true
	}
	if !reasons["duplicate target"] {
		t.Errorf("skipped reasons %v: want a duplicate entry", skipped)
	}
	foundBad := false
	for _, s := range skipped {
		if strings.Contains(s.Reason, "bad port") {
			foundBad = true
		}
	}
	if !foundBad {
		t.Errorf("skipped reasons %v: want a bad-port entry", skipped)
	}
}

func TestMergeFile(t *testing.T) {
	dst := &File{Categories: []Category{
		{Name: "cdn", Description: "CDN", Targets: []Target{{Host: "a.example", Port: 443, Method: "https"}}},
	}}
	src := &File{Categories: []Category{
		{Name: "cdn", Description: "CDN", Targets: []Target{
			{Host: "a.example", Port: 443, Method: "https"},
			{Host: "b.example", Port: 443, Method: "https"},
		}},
		{Name: "new", Description: "New", Targets: []Target{{Host: "c.example", Port: 0, Method: "icmp"}}},
	}}
	addedCat, addedTgt, skipped := MergeFile(dst, src)
	if addedCat != 1 || addedTgt != 2 || skipped != 1 {
		t.Fatalf("MergeFile = %d/%d/%d, want 1/2/1", addedCat, addedTgt, skipped)
	}
	if len(dst.Categories) != 2 || len(dst.Categories[0].Targets) != 2 {
		t.Fatalf("merged file = %+v", dst)
	}
}
