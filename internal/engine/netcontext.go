// Package engine — network context: proxy and VPN/tunnel detection.
//
// The detector is deliberately conservative and honest: it reports
// what it can see from the process environment (proxy variables and
// the -proxy flag) and from the network interfaces (names that look
// like tunnels, or point-to-point links). It cannot see a system
// proxy configured in the Windows registry or a VPN that hides its
// interface, so the results are labelled as a heuristic, not a
// guarantee.
package engine

import (
	"fmt"
	"net"
	"os"
	"strings"
)

// proxyEnvVars lists the proxy environment variables in priority
// order. The first non-empty one wins, matching the order the
// standard library checks them.
var proxyEnvVars = []string{
	"HTTPS_PROXY", "https_proxy",
	"HTTP_PROXY", "http_proxy",
	"ALL_PROXY", "all_proxy",
}

// tunnelKeywords are substrings (case-insensitive) that mark an
// interface name as a likely VPN/tunnel interface. This is a
// heuristic: names are a convention, not a guarantee.
var tunnelKeywords = []string{
	"tun", "tap", "wg", "wireguard", "ppp", "pptp", "l2tp", "sstp",
	"ipsec", "vpn", "utun", "tailscale", "zerotier", "openvpn",
	"forti", "anyconnect", "cisco", "globalprotect", "softether",
	"hamachi", "netbird", "nebula",
}

// NetworkContext describes the network the tests run through: any
// HTTP proxy in play and any VPN/tunnel interfaces that are up.
// The raw proxy URL is never printed: it may contain credentials.
// Use ProxySource for a safe label instead.
type NetworkContext struct {
	// ProxyEnv is the proxy URL from the environment (first
	// non-empty of proxyEnvVars), or "" when none is set.
	ProxyEnv string
	// ProxyFlag is the proxy URL from the -proxy flag, or "".
	ProxyFlag string
	// VPNInterfaces holds the names of up interfaces that look
	// like VPN/tunnel interfaces (heuristic, see looksLikeTunnel).
	VPNInterfaces []string

	// proxyEnvName is the environment variable ProxyEnv came from.
	proxyEnvName string
}

// DetectNetworkContext reads the proxy environment and lists the
// network interfaces. proxyFlag is the -proxy value ("" when the
// flag was not given); it wins over the environment for HTTP.
func DetectNetworkContext(proxyFlag string) NetworkContext {
	nc := NetworkContext{ProxyFlag: strings.TrimSpace(proxyFlag)}
	for _, name := range proxyEnvVars {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			nc.ProxyEnv = v
			nc.proxyEnvName = name
			break
		}
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return nc
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		if looksLikeTunnel(iface.Name, iface.Flags) {
			nc.VPNInterfaces = append(nc.VPNInterfaces, iface.Name)
		}
	}
	return nc
}

// ProxyActive reports whether any HTTP proxy is configured, by
// flag or by environment.
func (n NetworkContext) ProxyActive() bool {
	return n.ProxyFlag != "" || n.ProxyEnv != ""
}

// ProxySource names where the active proxy came from, without
// revealing the URL: "-proxy flag" or the environment variable
// name (e.g. "HTTPS_PROXY"), or "environment" as a fallback.
func (n NetworkContext) ProxySource() string {
	if n.ProxyFlag != "" {
		return "-proxy flag"
	}
	if n.proxyEnvName != "" {
		return n.proxyEnvName
	}
	if n.ProxyEnv != "" {
		return "environment"
	}
	return ""
}

// Notes returns the run-level notes for this context, in order:
// VPN first, then proxy. Empty when neither is present.
func (n NetworkContext) Notes() []string {
	var notes []string
	if len(n.VPNInterfaces) > 0 {
		notes = append(notes, fmt.Sprintf("VPN/tunnel interface detected (%s): measurements reflect the tunnel path.",
			strings.Join(n.VPNInterfaces, ", ")))
	}
	if n.ProxyActive() {
		notes = append(notes, fmt.Sprintf("HTTP proxy active (%s): HTTP via proxy; ping/TCP/DNS go direct, so results may differ.",
			n.ProxySource()))
	}
	return notes
}

// looksLikeTunnel reports whether an interface looks like a
// VPN/tunnel interface: its name contains a tunnel keyword, or it
// is a point-to-point link. Pure and unit-testable; the UP check
// lives in DetectNetworkContext, not here.
func looksLikeTunnel(name string, flags net.Flags) bool {
	if flags&net.FlagPointToPoint != 0 {
		return true
	}
	lower := strings.ToLower(name)
	for _, kw := range tunnelKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}
