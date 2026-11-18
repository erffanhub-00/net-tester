# net-tester

A network tester: ICMP ping, TCP connect, HTTP/HTTPS and real DNS resolution checks, with categories, scoring and ranking, and CSV + JSON export. One Go binary, no runtime to install — the same behaviour on Windows, Linux and macOS.

This is the merged rebuild of [Network-Ping-Pro](https://github.com/erffanhub-00/Network-Ping-Pro) (Python) and [server-tester](https://github.com/erffanhub-00/server-tester) (PowerShell): the useful core of both, with the extra features removed (no Stratum/mining support, no crypto-exchange presets, no fiddly HTTP knobs).

## Install

Download the binary for your system from the Releases page, or build it:

```
git clone https://github.com/erffanhub-00/net-tester.git
cd net-tester
go build -o net-tester ./cmd/net-tester
```

Requires Go 1.25+ to build. The binary itself needs nothing else — except the system `ping` program for ICMP tests (present on Windows, Linux and macOS).

## Quickstart

```
# Interactive menu: pick a category, run, see the ranking
./net-tester

# Test one category directly
./net-tester -no-menu -category cdn

# Several categories, 20 samples each
./net-tester -no-menu -category cdn,dns -packets 20

# Quiet, script-friendly output
./net-tester -no-menu -category dns -quiet
```

## What it tests

| Method | What it does |
| ------ | ------------ |
| `icmp` | Runs the system `ping`, N samples: avg/min/max, packet loss, jitter, P50/P95/P99, std dev |
| `tcp` | Opens a TCP connection to host:port, measures connect time. "Refused" means the host is up but the port is closed |
| `http` / `https` | GET request with status code, redirect following, status distribution, TTFB / connect / total times; HTTPS also validates the TLS certificate and times the handshake |
| `dns` | Asks the target resolver (e.g. 8.8.8.8) to resolve `example.com` and times the answer — a real DNS check, not a port-53 TCP connect |

## Scoring

Each target scores 0–100 (higher is better): latency 40%, packet loss 35%, jitter 25%, with penalties for a slow p95, a bad HTTP status, or an invalid TLS certificate. Grades: Excellent ≥90, Very Good ≥80, Good ≥70, Fair ≥60, Poor below that, Failed when nothing answered. Results are ranked best first; failed targets are listed last, unranked.

Jitter is the mean absolute difference between consecutive samples — the same definition both predecessor tools used, deliberately not RFC 3550 jitter.

## Categories

Categories live in `categories.json` next to the binary (edit it to add your own targets). If the file is missing, the built-in seed is used. A category is a name, a description and a list of targets; each target is one host + one port + one method:

```json
{
  "categories": [
    {
      "name": "cdn",
      "description": "CDN endpoints",
      "targets": [
        {"host": "cdn.jsdelivr.net", "port": 443, "method": "https"}
      ]
    }
  ]
}
```

Built-in categories: `cdn`, `dns`, `cloud`, `custom`. Duplicate targets (same host + port + method) are tested once, with a warning.

## Flags

```
-category string        Category to test (comma-separated names, or "all"); required with -no-menu
-packets int             Samples per target (1-200, default 10)
-config string           Categories file (default: ./categories.json, else the built-in seed)
-output-dir string       Directory for CSV and JSON exports (default "output")
-no-menu                 Run without the interactive menu (requires -category)
-workers int              Parallel targets (1-50, default 10)
-ping-timeout-ms int     Per-reply ping timeout in ms (100-60000, default 2000)
-tcp-timeout-ms int      TCP connect timeout in ms (100-60000, default 3000)
-http-timeout-ms int     HTTP request timeout in ms (100-60000, default 5000)
-dns-timeout-ms int      DNS lookup timeout in ms (100-60000, default 3000)
-proxy string            HTTP proxy URL (HTTP only)
-quiet                   One line per target, no table or chart
-version                 Print the version and exit
-help                    Show usage
```

## Output

Every run writes timestamped files to `output/`:

```
output/net-tester_cdn_20261118-183000.csv
output/net-tester_cdn_20261118-183000.json
```

The CSV has one row per target (25 columns: status, score, grade, latency stats, loss, resolved IP, per-stage timings, HTTP status, TLS validity). The JSON has the full results including the per-sample times.

## Exit codes

- `0` — every tested target was OK
- `1` — a target failed (or the export failed)
- `2` — usage error (bad flag, unknown category, `-no-menu` without `-category`)

## Interactive menu

```
net-tester — pick a category to test
  1. cdn — CDN endpoints (3 targets)
  2. dns — Public DNS resolvers (4 targets)
  3. cloud — Cloud provider endpoints (3 targets)
  4. custom — Example targets (2 targets)
  0. Exit
Category (number, 1,3,5 or 1-3, A for all, 0 to exit):
```

After a run you get the ranking table, an ASCII latency chart, a summary, and the option to view a per-target detail (latency bars per sample, per-stage timings, TLS info).

## Graphical interface

`net-tester-gui` is the Windows GUI (same engine as the CLI, native Windows controls, one .exe, no install). Double-click it — no console window opens.

**Test tab** — pick one or more categories and press Run. Results stream into the table as each target finishes (Target, Method, Status, Score, Avg, Loss, Jitter, P50, P95, Note), ranked best first, failed targets last and unranked. Settings in the window: packets (1-200), workers (1-50), ping/TCP/HTTP timeouts. Categories and targets are editable in the window (add/edit/delete targets, add/delete categories); changes are saved to `categories.json` next to the exe. **Nothing is exported automatically**: results live in the app until you press "Export CSV + JSON..." and pick a file.

**Live Ping tab** — enter a host and press Start. The system ping runs continuously (`ping -t` on Windows); every reply is parsed and shown live: a latency graph of the last 60 replies, stats (sent/received/loss, avg/min/max, jitter) and the latest replies. Stop kills the ping process.

## Proxy and VPN

net-tester notices what network it runs through and says so once per run (CLI: one line on stderr; GUI: a status line under the results):

- **HTTP proxy**: the `-proxy` flag wins; otherwise the proxy environment variables are honoured (`HTTPS_PROXY`/`HTTP_PROXY`/`ALL_PROXY`, first non-empty wins, `NO_PROXY` bypasses). Only HTTP(S) goes through the proxy — ping, TCP and DNS always go direct, so their results may differ from the HTTP ones.
- **VPN/tunnel**: interfaces that are up and look like a tunnel (name contains e.g. `tun`, `wg`, `tailscale`, `openvpn`, or a point-to-point link) are reported as "VPN/tunnel interface detected" — a heuristic, labelled as such. Measurements then reflect the tunnel path.
- **ICMP hint**: when a host fails on ping but answers on another method (TCP/HTTP/DNS), the result carries the note "host answers on other methods — ICMP may be blocked (e.g. by a VPN)".

The JSON export carries a top-level `"network"` object (`proxy_active`, `proxy_source`, `vpn_interfaces`, `notes`); the CSV rows are unchanged. The raw proxy URL is never printed or written — it may contain credentials.

What is *not* detected: a system proxy configured in the Windows registry (Windows Settings) is not read in v2 — only the environment variables and the `-proxy` flag.

## Development

```
go test ./...                       # full test suite
go build -o net-tester ./cmd/net-tester
GOOS=windows go build -o net-tester-windows-amd64.exe ./cmd/net-tester
GOOS=windows go build -trimpath -ldflags="-s -w -H windowsgui" -o net-tester-gui-windows-amd64.exe ./cmd/net-tester-gui
```

The GUI lives in `cmd/net-tester-gui` (Windows-only build tag; Linux/macOS builds skip it). The release exe embeds a visual-styles manifest (`cmd/net-tester-gui/net-tester-gui.manifest`, embedded with `go run github.com/akavel/rsrc -manifest cmd/net-tester-gui/net-tester-gui.manifest -o cmd/net-tester-gui/net-tester-gui.syso` before the build); builds without it look the same but use classic controls.

Tests cover scoring and grades, config validation, ping-output parsing (golden Windows, Linux and macOS outputs), TCP/HTTP/DNS against local test servers, the menu picker, and CSV/JSON export round-trips. Live internet tests are not part of the suite — the sandbox blocks raw sockets and intercepts TCP, so the ping engine is verified against golden outputs instead.

## License

MIT — see [LICENSE](LICENSE).

---

## Me

[![Telegram](https://img.shields.io/badge/Telegram-erffan_hub-blue?style=for-the-badge&logo=telegram)](https://t.me/erffan_hub) [![Twitter](https://img.shields.io/badge/Twitter-@Erffanhub_00-000000?style=for-the-badge&logo=x&logoColor=white)](https://x.com/Erffanhub_00) [![Gist](https://img.shields.io/badge/Gist-Profile-000000?style=for-the-badge&logo=gist)](https://gist.github.com/erffanhub-00) [![Github](https://img.shields.io/badge/Github-Profile-000000?style=for-the-badge&logo=github)](https://github.com/erffanhub-00)
