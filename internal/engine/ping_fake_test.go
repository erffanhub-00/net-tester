//go:build !windows

package engine

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/erffanhub-00/net-tester/internal/config"
)

// fakePingScript pretends to be the system ping binary. It prints
// canned output depending on the target host (last argument) so the
// whole ping path — spawn, parse, stats, status — can be tested
// end to end without raw sockets or a network. The win.test case
// prints Windows-dialect output even on Unix: the parser does not
// care about the arguments, only the text, so this also exercises
// the Windows parsing path through the real spawn flow.
const fakePingScript = `#!/bin/sh
host=""
for a in "$@"; do host="$a"; done
case "$host" in
good.test)
  echo "PING good.test (93.184.216.34) 56(84) bytes of data."
  echo "64 bytes from 93.184.216.34: icmp_seq=1 ttl=56 time=11.2 ms"
  echo "64 bytes from 93.184.216.34: icmp_seq=2 ttl=56 time=12.5 ms"
  echo "64 bytes from 93.184.216.34: icmp_seq=3 ttl=56 time=10.8 ms"
  echo "64 bytes from 93.184.216.34: icmp_seq=4 ttl=56 time=13.1 ms"
  echo ""
  echo "--- good.test ping statistics ---"
  echo "4 packets transmitted, 4 received, 0% packet loss, time 3004ms"
  exit 0
  ;;
lossy.test)
  echo "PING lossy.test (93.184.216.34) 56(84) bytes of data."
  echo "64 bytes from 93.184.216.34: icmp_seq=1 ttl=56 time=11.0 ms"
  echo "64 bytes from 93.184.216.34: icmp_seq=3 ttl=56 time=12.0 ms"
  echo ""
  echo "--- lossy.test ping statistics ---"
  echo "4 packets transmitted, 2 received, 50% packet loss, time 3004ms"
  exit 1
  ;;
dead.test)
  echo "PING dead.test (192.0.2.1) 56(84) bytes of data."
  echo ""
  echo "--- dead.test ping statistics ---"
  echo "4 packets transmitted, 0 received, 100% packet loss, time 3004ms"
  exit 1
  ;;
badhost.test)
  echo "ping: badhost.test: Name or service not known"
  exit 2
  ;;
win.test)
  echo "Pinging 8.8.8.8 with 32 bytes of data:"
  echo "Reply from 8.8.8.8: bytes=32 time=12ms TTL=117"
  echo "Reply from 8.8.8.8: bytes=32 time<1ms TTL=117"
  echo "Reply from 8.8.8.8: bytes=32 time=15ms TTL=117"
  echo "Request timed out."
  echo ""
  echo "Ping statistics for 8.8.8.8:"
  echo "    Packets: Sent = 4, Received = 3, Lost = 1 (25% loss),"
  exit 0
  ;;
*)
  echo "PING $host (127.0.0.1) 56(84) bytes of data."
  echo "64 bytes from 127.0.0.1: icmp_seq=1 ttl=64 time=0.1 ms"
  echo ""
  echo "--- $host ping statistics ---"
  echo "1 packets transmitted, 1 received, 0% packet loss, time 0ms"
  exit 0
  ;;
esac
`

func installFakePing(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ping"), []byte(fakePingScript), 0o755); err != nil {
		t.Fatalf("write fake ping: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestPingEndToEndFakeBinary(t *testing.T) {
	installFakePing(t)
	cases := []struct {
		host         string
		wantReceived int
		wantLoss     float64
		wantStatus   string
		wantAvg      float64
		wantIP       string
	}{
		{"good.test", 4, 0, StatusOK, 11.9, "93.184.216.34"},
		{"lossy.test", 2, 50, StatusHighLoss, 11.5, "93.184.216.34"},
		{"dead.test", 0, 100, StatusTimeout, 0, "192.0.2.1"},
		{"badhost.test", 0, 100, StatusDNSError, 0, ""},
		{"win.test", 3, 25, StatusHighLoss, 9.17, "8.8.8.8"},
	}
	for _, tc := range cases {
		r := ping(context.Background(), config.Target{Host: tc.host, Method: "icmp"}, 4, 2*time.Second)
		if r.Received != tc.wantReceived {
			t.Errorf("%s: received = %d, want %d (status %q, err %q)", tc.host, r.Received, tc.wantReceived, r.Status, r.Error)
		}
		if math.Abs(r.LossPct-tc.wantLoss) > 0.01 {
			t.Errorf("%s: loss = %v, want %v", tc.host, r.LossPct, tc.wantLoss)
		}
		if r.Status != tc.wantStatus {
			t.Errorf("%s: status = %q, want %q", tc.host, r.Status, tc.wantStatus)
		}
		if math.Abs(r.Avg-tc.wantAvg) > 0.01 {
			t.Errorf("%s: avg = %v, want %v", tc.host, r.Avg, tc.wantAvg)
		}
		if r.ResolvedIP != tc.wantIP {
			t.Errorf("%s: resolved IP = %q, want %q", tc.host, r.ResolvedIP, tc.wantIP)
		}
	}
}
