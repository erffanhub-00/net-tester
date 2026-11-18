package engine

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/erffanhub-00/net-tester/internal/config"
)

func testOptions() Options {
	o := DefaultOptions()
	o.Packets = 3
	o.PingTimeout = time.Second
	o.TCPTimeout = time.Second
	o.HTTPTimeout = 2 * time.Second
	o.DNSTimeout = time.Second
	return o
}

func startTCPEcho(t *testing.T) (int, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port, func() { ln.Close() }
}

func TestTCPTestLocalhost(t *testing.T) {
	port, closeFn := startTCPEcho(t)
	defer closeFn()
	r := tcpTest(context.Background(), config.Target{Host: "127.0.0.1", Port: port, Method: "tcp"}, 3, time.Second)
	if r.Status != StatusOK {
		t.Fatalf("status = %q (%s), want OK", r.Status, r.Error)
	}
	if r.Received != 3 || r.Score != 0 { // score is set by the runner, not the test
		t.Fatalf("received = %d, want 3", r.Received)
	}
	if r.ResolvedIP != "127.0.0.1" {
		t.Fatalf("resolved IP = %q", r.ResolvedIP)
	}
	if r.Avg <= 0 || r.Avg > 100 {
		t.Fatalf("avg = %v, want a small positive number", r.Avg)
	}
}

func TestTCPTestRefused(t *testing.T) {
	port, closeFn := startTCPEcho(t)
	closeFn() // nothing listening now
	r := tcpTest(context.Background(), config.Target{Host: "127.0.0.1", Port: port, Method: "tcp"}, 3, time.Second)
	if r.Status != StatusRefused {
		t.Fatalf("status = %q (%s), want Refused", r.Status, r.Error)
	}
	if r.Received != 0 {
		t.Fatalf("received = %d, want 0", r.Received)
	}
}

func TestIsRefusedWindowsError(t *testing.T) {
	// Windows (connectex) words it differently from Unix — the
	// classifier must recognise both, or the Windows CI run
	// reports Unreachable instead of Refused for a closed port.
	cases := []error{
		errors.New("dial tcp 127.0.0.1:54876: connectex: No connection could be made because the target machine actively refused it."),
		errors.New("dial tcp 127.0.0.1:54876: connect: connection refused"),
		&os.SyscallError{Syscall: "connect", Err: syscall.ECONNREFUSED},
	}
	for _, err := range cases {
		if !isRefused(err) {
			t.Fatalf("isRefused(%v) = false, want true", err)
		}
	}
	if isRefused(errors.New("dial tcp 127.0.0.1:54876: connect: connection timed out")) {
		t.Fatal("isRefused(timeout error) = true, want false")
	}
}

func TestTCPTestDNSError(t *testing.T) {
	// A cancelled context makes resolution fail deterministically,
	// whatever the sandbox DNS interceptor does with real names.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := tcpTest(ctx, config.Target{Host: "example.com", Port: 80, Method: "tcp"}, 2, time.Second)
	if r.Status != StatusDNSError {
		t.Fatalf("status = %q, want DNS Error", r.Status)
	}
	// Direct check: resolveHost reports the context error.
	if _, _, err := resolveHost(ctx, "example.com", time.Second); err == nil {
		t.Fatal("resolveHost with cancelled context should fail")
	}
}

func TestHTTPTestLocalhost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	hostport := strings.TrimPrefix(srv.URL, "http://")
	host, portStr, _ := net.SplitHostPort(hostport)
	p, err := net.LookupPort("tcp", portStr)
	if err != nil {
		t.Fatalf("port: %v", err)
	}
	port := p
	o := testOptions()
	r := httpTest(context.Background(), config.Target{Host: host, Port: port, Method: "http"}, o)
	if r.Status != StatusOK {
		t.Fatalf("status = %q (%s), want OK", r.Status, r.Error)
	}
	if r.HTTPStatus != 200 {
		t.Fatalf("HTTP status = %d, want 200", r.HTTPStatus)
	}
	if r.Received != 3 || r.TTFBMs <= 0 || r.ConnectMs <= 0 {
		t.Fatalf("received=%d ttfb=%v connect=%v", r.Received, r.TTFBMs, r.ConnectMs)
	}
	if len(r.HTTPDistribution) != 1 || r.HTTPDistribution[200] != 3 {
		t.Fatalf("distribution = %v", r.HTTPDistribution)
	}
}

func TestHTTPTest404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	hostport := strings.TrimPrefix(srv.URL, "http://")
	host, portStr, _ := net.SplitHostPort(hostport)
	p, _ := net.LookupPort("tcp", portStr)
	o := testOptions()
	r := httpTest(context.Background(), config.Target{Host: host, Port: p, Method: "http"}, o)
	if r.HTTPStatus != 404 {
		t.Fatalf("HTTP status = %d, want 404", r.HTTPStatus)
	}
	if got := r.DisplayStatus(); got != "HTTP 404" {
		t.Fatalf("DisplayStatus = %q, want HTTP 404", got)
	}
	// A 404 still counts as a sample: the server answered.
	if r.Received != 3 {
		t.Fatalf("received = %d, want 3 (404 is an answer)", r.Received)
	}
}

// fakeDNSServer answers every A query with 93.184.216.34.
// NOTE: this server cannot be used for an end-to-end test in this
// sandbox — the sandbox drops UDP replies to ephemeral ports, so a
// loopback DNS server can never answer here. The builder is
// unit-tested below (TestBuildDNSResponse) and the live success
// path is covered by TestDNSTestSystemResolver instead.
func buildDNSResponse(q []byte) []byte {
	if len(q) < 12 {
		return nil
	}
	// Walk the question to find its end (QNAME labels + QTYPE + QCLASS).
	i := 12
	for i < len(q) && q[i] != 0 {
		i += 1 + int(q[i])
	}
	i++ // null label
	if i+4 > len(q) {
		return nil
	}
	qEnd := i + 4
	qtype := uint16(q[qEnd-4])<<8 | uint16(q[qEnd-3])
	resp := make([]byte, 0, qEnd+16)
	// Header: ID, flags 0x8180 (response, no error), QD=1, AN=1 or 0, NS=0, AR=0.
	anCount := 1
	if qtype != 1 {
		anCount = 0 // no A record for non-A queries (e.g. AAAA): empty NOERROR
	}
	resp = append(resp, q[0], q[1], 0x81, 0x80, 0x00, 0x01, 0x00, byte(anCount), 0x00, 0x00, 0x00, 0x00)
	// Question section, copied verbatim.
	resp = append(resp, q[12:qEnd]...)
	if anCount == 0 {
		return resp
	}
	// Answer: name pointer to offset 12, type A, class IN, TTL 60, 4-byte RDATA.
	resp = append(resp, 0xC0, 0x0C, 0x00, 0x01, 0x00, 0x01)
	ttl := make([]byte, 4)
	binary.BigEndian.PutUint32(ttl, 60)
	resp = append(resp, ttl...)
	resp = append(resp, 0x00, 0x04, 93, 184, 216, 34)
	return resp
}

func dnsQuery(id uint16, qtype uint16) []byte {
	q := []byte{byte(id >> 8), byte(id), 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x07, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 0x03, 'c', 'o', 'm', 0x00,
		byte(qtype >> 8), byte(qtype), 0x00, 0x01}
	return q
}

func TestBuildDNSResponse(t *testing.T) {
	// A query: 45-byte response, ID echoed, one A record.
	resp := buildDNSResponse(dnsQuery(0x1234, 1))
	if len(resp) != 45 {
		t.Fatalf("A response length = %d, want 45", len(resp))
	}
	if resp[0] != 0x12 || resp[1] != 0x34 {
		t.Fatalf("ID not echoed: %x", resp[:2])
	}
	if resp[2] != 0x81 || resp[3] != 0x80 {
		t.Fatalf("flags = %x %x, want 81 80", resp[2], resp[3])
	}
	if resp[6] != 0x00 || resp[7] != 0x01 {
		t.Fatalf("ANCOUNT = %d, want 1", resp[7])
	}
	if got := resp[len(resp)-4:]; got[0] != 93 || got[1] != 184 || got[2] != 216 || got[3] != 34 {
		t.Fatalf("RDATA = %v, want 93.184.216.34", got)
	}
	// AAAA query: empty NOERROR (ANCOUNT 0, question echoed).
	resp = buildDNSResponse(dnsQuery(0x1234, 28))
	if len(resp) != 29 || resp[7] != 0x00 {
		t.Fatalf("AAAA response: len=%d ANCOUNT=%d, want 29/0", len(resp), resp[7])
	}
	if buildDNSResponse([]byte{1, 2, 3}) != nil {
		t.Fatal("short input should return nil")
	}
}

// systemResolver reads the first nameserver from /etc/resolv.conf.
// It returns "" when none is configured (e.g. on Windows).
func systemResolver() string {
	data, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && f[0] == "nameserver" {
			return f[1]
		}
	}
	return ""
}

func TestDNSTestSystemResolver(t *testing.T) {
	ns := systemResolver()
	if ns == "" {
		t.Skip("no nameserver in /etc/resolv.conf (Windows?)")
	}
	o := testOptions()
	r := dnsTest(context.Background(), config.Target{Host: ns, Method: "dns"}, o)
	if r.Status != StatusOK {
		t.Fatalf("status = %q (%s), want OK (resolver %s)", r.Status, r.Error, ns)
	}
	if r.Received != 3 || r.ResolvedIP == "" {
		t.Fatalf("received=%d resolved=%q", r.Received, r.ResolvedIP)
	}
	if r.DNSMs <= 0 {
		t.Fatalf("DNS time = %v, want positive", r.DNSMs)
	}
}

func TestDNSTestDeadResolver(t *testing.T) {
	// A UDP server that swallows queries without answering: the
	// lookups must time out (a closed loopback port instead
	// answers with ICMP "connection refused", which is an instant
	// error, not a timeout).
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	defer pc.Close()
	go func() {
		buf := make([]byte, 512)
		for {
			if _, _, err := pc.ReadFrom(buf); err != nil {
				return
			}
			// Never reply: black hole.
		}
	}()
	port := pc.LocalAddr().(*net.UDPAddr).Port
	o := testOptions()
	o.DNSTimeout = 300 * time.Millisecond
	r := dnsTest(context.Background(), config.Target{Host: "127.0.0.1", Port: port, Method: "dns"}, o)
	if r.Status != StatusTimeout {
		t.Fatalf("status = %q (%s), want Timeout", r.Status, r.Error)
	}
}

func TestRunTargetsRankingAndDedupe(t *testing.T) {
	port, closeFn := startTCPEcho(t)
	defer closeFn()
	targets := []config.Target{
		{Host: "127.0.0.1", Port: port, Method: "tcp"},
		{Host: "127.0.0.1", Port: port, Method: "tcp"}, // duplicate
		{Host: "127.0.0.1", Port: 1, Method: "tcp"},    // refused (port 1 closed)
	}
	results := RunTargets(context.Background(), targets, testOptions())
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2 (duplicate tested once)", len(results))
	}
	if results[0].Status != StatusOK || results[0].Rank != 1 {
		t.Fatalf("first result: status=%q rank=%d, want OK/1", results[0].Status, results[0].Rank)
	}
	if results[1].Status != StatusRefused || results[1].Rank != 0 {
		t.Fatalf("second result: status=%q rank=%d, want Refused/0", results[1].Status, results[1].Rank)
	}
	if results[0].Score <= 0 {
		t.Fatalf("good target score = %v, want > 0", results[0].Score)
	}
	if results[1].Score != 0 || results[1].Grade != "Failed" {
		t.Fatalf("failed target: score=%v grade=%q, want 0/Failed", results[1].Score, results[1].Grade)
	}
}
