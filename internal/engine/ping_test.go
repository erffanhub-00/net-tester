package engine

import (
	"strings"
	"testing"
)

const windowsPingOK = `Pinging 8.8.8.8 with 32 bytes of data:
Reply from 8.8.8.8: bytes=32 time=12ms TTL=117
Reply from 8.8.8.8: bytes=32 time=11ms TTL=117
Reply from 8.8.8.8: bytes=32 time=13ms TTL=117
Reply from 8.8.8.8: bytes=32 time<1ms TTL=117

Ping statistics for 8.8.8.8:
    Packets: Sent = 4, Received = 4, Lost = 0 (0% loss),
Approximate round trip times in milli-seconds:
    Minimum = 0ms, Maximum = 13ms, Average = 9ms
`

const windowsPingLoss = `Pinging 192.0.2.1 with 32 bytes of data:
Reply from 192.0.2.1: bytes=32 time=21ms TTL=64
Request timed out.
Reply from 192.0.2.1: bytes=32 time=19ms TTL=64
Request timed out.

Ping statistics for 192.0.2.1:
    Packets: Sent = 4, Received = 2, Lost = 2 (50% loss),
Approximate round trip times in milli-seconds:
    Minimum = 19ms, Maximum = 21ms, Average = 20ms
`

const windowsPingUnreachable = `Pinging 10.255.255.1 with 32 bytes of data:
Reply from 10.0.0.5: Destination host unreachable.
Reply from 10.0.0.5: Destination host unreachable.
Reply from 10.0.0.5: Destination host unreachable.
Reply from 10.0.0.5: Destination host unreachable.

Ping statistics for 10.255.255.1:
    Packets: Sent = 4, Received = 0, Lost = 4 (100% loss),
`

const windowsPingDNSFail = `Ping request could not find host nonexistent.invalid. Please check the name and try again.
`

const linuxPingOK = `PING 8.8.8.8 (8.8.8.8) 56(84) bytes of data.
64 bytes from 8.8.8.8: icmp_seq=1 ttl=117 time=11.2 ms
64 bytes from 8.8.8.8: icmp_seq=2 ttl=117 time=10.8 ms
64 bytes from 8.8.8.8: icmp_seq=3 ttl=117 time=12.4 ms
64 bytes from 8.8.8.8: icmp_seq=4 ttl=117 time=11.9 ms

--- 8.8.8.8 ping statistics ---
4 packets transmitted, 4 received, 0% packet loss, time 3002ms
rtt min/avg/max/mdev = 10.800/11.575/12.400/0.589 ms
`

const linuxPingLoss = `PING 192.0.2.1 (192.0.2.1) 56(84) bytes of data.
64 bytes from 192.0.2.1: icmp_seq=1 ttl=64 time=21.3 ms
64 bytes from 192.0.2.1: icmp_seq=3 ttl=64 time=19.7 ms

--- 192.0.2.1 ping statistics ---
4 packets transmitted, 2 received, 50% packet loss, time 3003ms
rtt min/avg/max/mdev = 19.700/20.500/21.300/0.800 ms
`

const linuxPingDNSFail = `ping: nonexistent.invalid: Name or service not known
`

const macPingOK = `PING 8.8.8.8 (8.8.8.8): 56 data bytes
64 bytes from 8.8.8.8: icmp_seq=0 ttl=117 time=11.234 ms
64 bytes from 8.8.8.8: icmp_seq=1 ttl=117 time=10.456 ms
64 bytes from 8.8.8.8: icmp_seq=2 ttl=117 time=12.789 ms

--- 8.8.8.8 ping statistics ---
3 packets transmitted, 3 packets received, 0.0% packet loss
round-trip min/avg/max/stddev = 10.456/11.493/12.789/0.956 ms
`

func TestParsePingWindowsOK(t *testing.T) {
	samples, sent, received, dnsErr := parsePingOutput(windowsPingOK, 4)
	if dnsErr {
		t.Fatal("unexpected dnsErr")
	}
	if sent != 4 || received != 4 {
		t.Fatalf("sent=%d received=%d, want 4/4", sent, received)
	}
	if len(samples) != 4 {
		t.Fatalf("got %d samples, want 4: %v", len(samples), samples)
	}
	// 12, 11, 13, 0.5 for the "<1ms" reply.
	if samples[3] != 0.5 {
		t.Fatalf("last sample = %v, want 0.5 (time<1ms)", samples[3])
	}
	if got := parsePingIP(windowsPingOK); got != "8.8.8.8" {
		t.Fatalf("parsePingIP = %q, want 8.8.8.8", got)
	}
}

func TestParsePingWindowsLoss(t *testing.T) {
	samples, sent, received, _ := parsePingOutput(windowsPingLoss, 4)
	if sent != 4 || received != 2 {
		t.Fatalf("sent=%d received=%d, want 4/2", sent, received)
	}
	if len(samples) != 2 || samples[0] != 21 || samples[1] != 19 {
		t.Fatalf("samples = %v, want [21 19]", samples)
	}
}

func TestParsePingWindowsUnreachable(t *testing.T) {
	_, _, received, dnsErr := parsePingOutput(windowsPingUnreachable, 4)
	if dnsErr {
		t.Fatal("unexpected dnsErr")
	}
	if received != 0 {
		t.Fatalf("received=%d, want 0", received)
	}
}

func TestParsePingDNSFail(t *testing.T) {
	for _, out := range []string{windowsPingDNSFail, linuxPingDNSFail} {
		_, _, _, dnsErr := parsePingOutput(out, 4)
		if !dnsErr {
			t.Fatalf("expected dnsErr for %q", strings.Split(out, "\n")[0])
		}
	}
}

func TestParsePingLinuxOK(t *testing.T) {
	samples, sent, received, dnsErr := parsePingOutput(linuxPingOK, 4)
	if dnsErr {
		t.Fatal("unexpected dnsErr")
	}
	if sent != 4 || received != 4 {
		t.Fatalf("sent=%d received=%d, want 4/4", sent, received)
	}
	if len(samples) != 4 || samples[0] != 11.2 || samples[2] != 12.4 {
		t.Fatalf("samples = %v", samples)
	}
	if got := parsePingIP(linuxPingOK); got != "8.8.8.8" {
		t.Fatalf("parsePingIP = %q, want 8.8.8.8", got)
	}
}

func TestParsePingLinuxLoss(t *testing.T) {
	samples, sent, received, _ := parsePingOutput(linuxPingLoss, 4)
	if sent != 4 || received != 2 {
		t.Fatalf("sent=%d received=%d, want 4/2", sent, received)
	}
	if len(samples) != 2 {
		t.Fatalf("samples = %v, want 2", samples)
	}
}

func TestParsePingMacOK(t *testing.T) {
	samples, sent, received, dnsErr := parsePingOutput(macPingOK, 3)
	if dnsErr {
		t.Fatal("unexpected dnsErr")
	}
	if sent != 3 || received != 3 {
		t.Fatalf("sent=%d received=%d, want 3/3", sent, received)
	}
	if len(samples) != 3 || samples[0] != 11.234 {
		t.Fatalf("samples = %v", samples)
	}
}

func TestSummarise(t *testing.T) {
	s := Summarise([]float64{10, 20, 30, 40})
	if s.Avg != 25 || s.Min != 10 || s.Max != 40 {
		t.Fatalf("avg/min/max = %v/%v/%v", s.Avg, s.Min, s.Max)
	}
	// Jitter: |20-10|+|30-20|+|40-30| = 30 / 3 = 10.
	if s.Jitter != 10 {
		t.Fatalf("jitter = %v, want 10", s.Jitter)
	}
	if s.P50 != 25 || s.P95 != 38.5 {
		t.Fatalf("p50/p95 = %v/%v, want 25/38.5", s.P50, s.P95)
	}
	if s.StdDev == 0 {
		t.Fatal("stddev should be non-zero")
	}
	if got := Summarise(nil); got.Avg != 0 || got.P95 != 0 {
		t.Fatalf("empty samples should give zero stats, got %+v", got)
	}
}

func TestLossStatus(t *testing.T) {
	if LossStatus(0) != StatusOK || LossStatus(10) != StatusPartial || LossStatus(20) != StatusHighLoss || LossStatus(100) != StatusHighLoss {
		t.Fatal("wrong loss status mapping")
	}
}
