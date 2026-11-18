# net-tester

A graphical network tester for Windows: ICMP ping, TCP connect, HTTP/HTTPS and real DNS resolution checks, with categories, scoring, a ranking table, and CSV + JSON export on demand. One .exe, no install, no console window.

This is the merged rebuild of [Network-Ping-Pro](https://github.com/erffanhub-00/Network-Ping-Pro) (Python) and [server-tester](https://github.com/erffanhub-00/server-tester) (PowerShell): the useful core of both, with the extra features removed (no Stratum/mining support, no crypto-exchange presets, no fiddly HTTP knobs). Version 2.2.0 is GUI-only — there is no command-line version anymore.

## Run

Download `net-tester-gui-windows-amd64.exe` from the Releases page and double-click it. It needs nothing else — except the system `ping` program for ICMP tests (present on Windows). The ping runs hidden: no terminal window opens while testing.

## Test tab

Pick one or more categories and press **Run**. Results stream into the table as each target finishes (Target, Method, Status, Score, Avg, Loss, Jitter, P50, P95, Note), ranked best first, failed targets last and unranked. **Stop** cancels the run. The table scrolls; so does the whole tab when the window is small.

Settings in the window: packets (1-200), workers (1-50), ping/TCP/HTTP timeouts.

Categories and targets are editable in the window (add/edit/delete targets, add/delete categories); changes are saved to `categories.json` next to the exe. **Nothing is exported automatically**: results live in the app until you press **Export CSV + JSON...** and pick a file.

## Live Ping tab

Enter a host and press **Start**. The system ping runs continuously (`ping -t`) and every reply is parsed and shown live:

- **Current: 12 ms** — the latest ping, big, at the top (`—` before the first reply, `timeout` when a reply is missed)
- stats (sent/received/loss, avg/min/max, jitter)
- a latency graph of the last 60 replies
- the latest replies as a scrolling list

**Stop** kills the ping process. The ping runs hidden — no terminal window opens.

## Ranking tab

A leaderboard of the last run: Rank | Target | Method | Score | Grade | Avg ms — best first, failed targets last as unranked. It updates after every run; before the first run it says "Run a test first."

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

## Categories and bulk import

Categories live in `categories.json` next to the exe (edit it to add your own targets, or edit them in the window). If the file is missing, the built-in seed is used. A category is a name, a description and a list of targets; each target is one host + one port + one method:

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

Built-in categories: `cdn`, `dns`, `cloud`, `custom`. Duplicate targets (same host + port + method) are tested once.

**Bulk import** — press **Import...** in the Test tab to add many targets at once. Pick the destination category (or type a new name), then paste targets, one per line:

```
example.com
example.com:443
1.1.1.1:53 dns
google.com https
```

Line format: `host`, `host:port`, `host method`, `host:port method`. Blank lines and lines starting with `#` are ignored. Defaults: no port → `icmp`; port given → `tcp`; port defaults by method are icmp/dns → 0, http → 80, https/tcp → 443. After the import you get a summary: how many were added, and each skipped line with its reason (bad line, duplicate).

**Import from file...** in the same dialog: a `.txt` file is loaded into the text box (same line format); a `.json` file is a categories file and is merged right away — new categories are added, new targets are appended to existing ones, duplicates are skipped.

## Export

Nothing is written to disk unless you press **Export CSV + JSON...** and pick a file. It writes two files next to each other (same name, `.csv` and `.json`).

The CSV has one row per target (25 columns: status, score, grade, latency stats, loss, resolved IP, per-stage timings, HTTP status, TLS validity). The JSON has the full results including the per-sample times.

## Proxy and VPN

net-tester notices what network it runs through and says so in a status line under the results:

- **HTTP proxy**: the proxy environment variables are honoured (`HTTPS_PROXY`/`HTTP_PROXY`/`ALL_PROXY`, first non-empty wins, `NO_PROXY` bypasses). Only HTTP(S) goes through the proxy — ping, TCP and DNS always go direct, so their results may differ from the HTTP ones.
- **VPN/tunnel**: interfaces that are up and look like a tunnel (name contains e.g. `tun`, `wg`, `tailscale`, `openvpn`, or a point-to-point link) are reported as "VPN/tunnel interface detected" — a heuristic, labelled as such. Measurements then reflect the tunnel path.
- **ICMP hint**: when a host fails on ping but answers on another method (TCP/HTTP/DNS), the result carries the note "host answers on other methods — ICMP may be blocked (e.g. by a VPN)".

The JSON export carries a top-level `"network"` object (`proxy_active`, `proxy_source`, `vpn_interfaces`, `notes`); the CSV rows are unchanged. The raw proxy URL is never printed or written — it may contain credentials.

What is *not* detected: a system proxy configured in the Windows registry (Windows Settings) is not read — only the environment variables.

## Build from source

Requires Go 1.25+. The GUI is Windows-only (`//go:build windows`); Linux and macOS builds skip it.

```
git clone https://github.com/erffanhub-00/net-tester.git
cd net-tester

# Windows GUI exe (from any platform, cross-compiled):
go run github.com/akavel/rsrc@v0.10.2 -manifest cmd/net-tester-gui/net-tester-gui.manifest -o cmd/net-tester-gui/rsrc.syso
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -H windowsgui" -o net-tester-gui-windows-amd64.exe ./cmd/net-tester-gui
```

The `rsrc.syso` step embeds the visual-styles manifest (modern Windows controls). It is generated, not committed — a build without it still works but uses classic controls.

## Development

```
go test ./...    # full test suite
go vet ./...     # Linux and Windows: GOOS=windows go vet ./...
go build ./...   # builds everything except the Windows-only GUI package on non-Windows
```

The engine (`internal/engine`, `internal/config`, `internal/score`, `internal/report`) is stdlib-only and cross-platform; the GUI (`cmd/net-tester-gui`) uses [lxn/walk](https://github.com/lxn/walk) (pure Go, no CGO, native Windows controls).

Tests cover scoring and grades, config validation and bulk-import parsing, ping-output parsing (golden Windows, Linux and macOS outputs), TCP/HTTP/DNS against local test servers, and CSV/JSON export round-trips. Live internet tests are not part of the suite — the sandbox blocks raw sockets and intercepts TCP, so the ping engine is verified against golden outputs instead, and the GUI itself is never executed here.

## License

MIT — see [LICENSE](LICENSE).

---

## Me

[![Telegram](https://img.shields.io/badge/Telegram-erffan_hub-blue?style=for-the-badge&logo=telegram)](https://t.me/erffan_hub) [![Twitter](https://img.shields.io/badge/Twitter-@Erffanhub_00-000000?style=for-the-badge&logo=x&logoColor=white)](https://x.com/Erffanhub_00) [![Gist](https://img.shields.io/badge/Gist-Profile-000000?style=for-the-badge&logo=gist)](https://gist.github.com/erffanhub-00) [![Github](https://img.shields.io/badge/Github-Profile-000000?style=for-the-badge&logo=github)](https://github.com/erffanhub-00)
