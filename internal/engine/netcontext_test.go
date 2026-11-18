package engine

import (
	"net"
	"net/http"
	"net/url"
	"runtime"
	"testing"
)

func TestLooksLikeTunnel(t *testing.T) {
	up := net.FlagUp
	cases := []struct {
		name  string
		flags net.Flags
		want  bool
	}{
		{"tun0", up, true},
		{"wg0", up, true},
		{"utun3", up, true},
		{"tailscale0", up, true},
		{"ppp0", up, true},
		{"FortiClient", up, true},
		{"Cisco AnyConnect", up, true},
		{"zerotier-one", up, true},
		{"eth0", up, false},
		{"en0", up, false},
		{"Wi-Fi", up, false},
		{"lo", net.FlagUp | net.FlagLoopback, false},
		// A plain name on a point-to-point link still counts: the
		// link type is the signal, not the name.
		{"eth0", up | net.FlagPointToPoint, true},
		// Down interfaces never reach looksLikeTunnel from the
		// detector, but the function itself only judges name/flags.
		{"tun0", 0, true},
	}
	for _, c := range cases {
		if got := looksLikeTunnel(c.name, c.flags); got != c.want {
			t.Errorf("looksLikeTunnel(%q, %v) = %v, want %v", c.name, c.flags, got, c.want)
		}
	}
}

func clearProxyEnv(t *testing.T) {
	t.Helper()
	for _, name := range append(append([]string{}, proxyEnvVars...), "NO_PROXY", "no_proxy") {
		t.Setenv(name, "")
	}
}

func TestDetectNetworkContextEnv(t *testing.T) {
	clearProxyEnv(t)
	t.Setenv("HTTPS_PROXY", "http://proxy.example:8080")
	t.Setenv("HTTP_PROXY", "http://other.example:3128")
	nc := DetectNetworkContext("")
	if nc.ProxyEnv != "http://proxy.example:8080" {
		t.Fatalf("ProxyEnv = %q, want the HTTPS_PROXY value (first non-empty wins)", nc.ProxyEnv)
	}
	if nc.ProxySource() != "HTTPS_PROXY" {
		t.Fatalf("ProxySource = %q, want HTTPS_PROXY", nc.ProxySource())
	}
	if !nc.ProxyActive() {
		t.Fatal("ProxyActive should be true with an env proxy")
	}
}

func TestDetectNetworkContextFlagWins(t *testing.T) {
	clearProxyEnv(t)
	t.Setenv("HTTPS_PROXY", "http://proxy.example:8080")
	nc := DetectNetworkContext("http://flag.example:3128")
	if nc.ProxyFlag != "http://flag.example:3128" {
		t.Fatalf("ProxyFlag = %q", nc.ProxyFlag)
	}
	if nc.ProxySource() != "-proxy flag" {
		t.Fatalf("ProxySource = %q, want -proxy flag", nc.ProxySource())
	}
}

func TestDetectNetworkContextNone(t *testing.T) {
	clearProxyEnv(t)
	nc := DetectNetworkContext("")
	if nc.ProxyActive() || nc.ProxyEnv != "" || nc.ProxyFlag != "" {
		t.Fatalf("expected no proxy, got %+v", nc)
	}
	if len(nc.Notes()) != 0 {
		t.Fatalf("expected no notes, got %v", nc.Notes())
	}
}

func TestNetworkNotes(t *testing.T) {
	nc := NetworkContext{
		ProxyEnv:      "http://proxy.example:8080",
		VPNInterfaces: []string{"tun0", "wg0"},
		proxyEnvName:  "HTTPS_PROXY",
	}
	notes := nc.Notes()
	if len(notes) != 2 {
		t.Fatalf("got %d notes, want 2: %v", len(notes), notes)
	}
	if notes[0] != "VPN/tunnel interface detected (tun0, wg0): measurements reflect the tunnel path." {
		t.Errorf("VPN note = %q", notes[0])
	}
	if notes[1] != "HTTP proxy active (HTTPS_PROXY): HTTP via proxy; ping/TCP/DNS go direct, so results may differ." {
		t.Errorf("proxy note = %q", notes[1])
	}
}

func TestProxyFromEnv(t *testing.T) {
	clearProxyEnv(t)
	t.Setenv("HTTPS_PROXY", "http://proxy.example:8080")

	req := &http.Request{URL: &url.URL{Scheme: "https", Host: "example.com"}}
	u, err := proxyFromEnv(req)
	if err != nil {
		t.Fatalf("proxyFromEnv: %v", err)
	}
	if u == nil || u.Host != "proxy.example:8080" {
		t.Fatalf("proxy = %v, want http://proxy.example:8080", u)
	}

	// HTTP scheme uses HTTP_PROXY, not HTTPS_PROXY.
	req.URL = &url.URL{Scheme: "http", Host: "example.com"}
	u, err = proxyFromEnv(req)
	if err != nil || u != nil {
		t.Fatalf("http URL with only HTTPS_PROXY set: proxy = %v, want none", u)
	}

	// NO_PROXY bypasses the proxy.
	t.Setenv("NO_PROXY", "example.com")
	req.URL = &url.URL{Scheme: "https", Host: "example.com"}
	u, err = proxyFromEnv(req)
	if err != nil || u != nil {
		t.Fatalf("NO_PROXY host: proxy = %v, want none", u)
	}

	// Loopback is always direct.
	t.Setenv("NO_PROXY", "")
	req.URL = &url.URL{Scheme: "https", Host: "127.0.0.1:8080"}
	u, err = proxyFromEnv(req)
	if err != nil || u != nil {
		t.Fatalf("loopback: proxy = %v, want none", u)
	}
}

func TestProxyForFlagOverridesEnv(t *testing.T) {
	clearProxyEnv(t)
	t.Setenv("HTTPS_PROXY", "http://proxy.example:8080")
	fn := proxyFor(Options{Proxy: "http://flag.example:3128"})
	req := &http.Request{URL: &url.URL{Scheme: "https", Host: "example.com"}}
	u, err := fn(req)
	if err != nil {
		t.Fatalf("proxyFor(flag): %v", err)
	}
	if u == nil || u.Host != "flag.example:3128" {
		t.Fatalf("proxy = %v, want the flag proxy", u)
	}

	// Empty flag falls through to the environment.
	fn = proxyFor(Options{})
	u, err = fn(req)
	if err != nil {
		t.Fatalf("proxyFor(env): %v", err)
	}
	if u == nil || u.Host != "proxy.example:8080" {
		t.Fatalf("proxy = %v, want the env proxy", u)
	}

	// An invalid flag URL disables the proxy instead of failing.
	fn = proxyFor(Options{Proxy: "://bad url"})
	if fn != nil {
		t.Fatal("invalid flag proxy should give a nil proxy func")
	}
}

func TestLivePingArgs(t *testing.T) {
	args := LivePingArgs("example.com")
	if len(args) == 0 || args[len(args)-1] != "example.com" {
		t.Fatalf("args = %v, want the host last", args)
	}
	if runtime.GOOS == "windows" {
		if len(args) != 2 || args[0] != "-t" {
			t.Fatalf("windows args = %v, want [-t host]", args)
		}
	}
}
