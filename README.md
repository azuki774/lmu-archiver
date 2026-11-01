# lmu-archiver

A command-line tool for Le Mans Ultimate, intended to retrieve and archive your
online racing data under your own control.

## Status

**Work in progress.** The Go CLI can obtain a Steam session ticket from a running
LMU instance and authenticate with RaceOS (`raceos.gg`). Specific data retrieval,
processing, and archiving are not implemented yet.

## Goals and data ownership

Unlike companion services that sync racing data to their own hosted accounts,
lmu-archiver aims to let you keep ownership and control of your data: where it is
stored, how it is used, and whether it is shared.

The goal is local archiving without a required third-party dashboard, analytics,
or cloud synchronization. This does not remove the need to contact the game's
online services to retrieve your data.

## Authentication reference

The authentication flow is informed by
[LMU Central](https://github.com/MarteauPiqueurdu11/LMU-Central), especially its
[RaceOS client](https://github.com/MarteauPiqueurdu11/LMU-Central/blob/main/lmu_central/core/raceos.py)
and
[technical notes](https://github.com/MarteauPiqueurdu11/LMU-Central/blob/main/docs/notes-techniques.md).
These are implementation references, not runtime dependencies.

See the [workflow](docs/workflow.md) / [ワークフロー](docs/workflow.ja.md) for an
overview of the authentication-to-archiving flow.

This project is independent and is not affiliated with Le Mans Ultimate,
Studio 397, Motorsport Games, or the referenced tools.

## API request pacing

The CLI shares one HTTP client across LMU and RaceOS requests. The first request
can start immediately; subsequent requests wait at least one second after the
previous transport attempt returns response headers or an error. Concurrent
callers share the same gate, with no accumulated burst allowance after idle time.
Response bodies do not hold the gate.

Waiting is cancelable and counts toward the client's 20-second request timeout.
Failed attempts also consume the cooldown; HTTP errors, including 429, are not
automatically retried. The existing 1.2-second authentication-specific wait remains.
New API clients must receive the same HTTP client to participate in this limit.
Separate CLI processes, underlying transport retries, and server arrival timing
are outside this guarantee.

## Project layout

```text
cmd/lmu-archiver/   CLI entry point
internal/auth/     LMU ticket acquisition and RaceOS authentication
internal/httpclient/ Shared HTTP client and API request cooldown
bin/               Local build output (ignored except for .gitkeep)
go.mod             Go module definition
Makefile           Local build, run, and test commands
Dockerfile         Multi-stage container build
```

## Local environment

Copy the reference configuration and adjust the LMU internal API URL for your
environment:

```sh
cp .env.sample .env
```

The sample uses the local Windows API address:

```dotenv
LMU_INT_API="http://127.0.0.1:6937/"
```

`.env` is ignored by Git; `.env.sample` is the shared reference. The current CLI
does not load `.env` or use `LMU_INT_API` automatically. After building, load your
trusted local file in a POSIX shell and pass its value explicitly:

```sh
. ./.env
./bin/lmu-archiver auth --lmu-url "$LMU_INT_API"
```

### Accessing the Windows API from another machine (at your own risk)

If the API listens only on `127.0.0.1:6937`, another machine cannot connect to it
directly. The example below forwards `<Windows LAN IP>:16397` to
`127.0.0.1:6937` on the Windows machine running LMU.

**Use at your own risk.** This exposes a loopback-only API to the network.
`portproxy` adds neither authentication nor TLS; use only a trusted LAN and do not
expose this port to the Internet or configure router port forwarding.

Open **PowerShell as Administrator** on the Windows machine. Replace both
placeholders with actual IPv4 addresses: `$lmuHostIp` is the Windows machine's LAN
IP, and `$clientIp` is the allowed connecting machine's IP as seen by Windows.
This example assumes that the trusted Windows LAN connection uses the **Private**
network profile; the firewall rule will not apply on a Public connection.

```powershell
$lmuHostIp = "<Windows LAN IP>"
$clientIp = "<allowed client IP>"

netsh interface portproxy add v4tov4 `
    listenaddress=$lmuHostIp `
    listenport=16397 `
    connectaddress=127.0.0.1 `
    connectport=6937

New-NetFirewallRule `
    -DisplayName "LMU REST proxy 16397" `
    -Direction Inbound `
    -Protocol TCP `
    -LocalAddress $lmuHostIp `
    -LocalPort 16397 `
    -RemoteAddress $clientIp `
    -Profile Private `
    -Action Allow
```

The proxy listens only on the selected Windows IP. The firewall rule allows TCP
port `16397` only for that local IP and the specified client on the Private
profile. Review any existing firewall rules as well; this rule does not override
broader permissions already configured.

For example, if the Windows LAN IP is `192.168.10.16`, use this in the connecting
machine's `.env`:

```dotenv
LMU_INT_API="http://192.168.10.16:16397"
```

With LMU and its API running, inspect the proxy on Windows using
`netsh interface portproxy show v4tov4`. From the allowed client, check the HTTP
connection with `curl --verbose --max-time 5 http://192.168.10.16:16397/`
(substitute your Windows IP). An HTTP error at `/` can still indicate that the
connection works; it does not prove a particular API endpoint is available.

References: Microsoft documentation for
[`netsh interface portproxy`](https://learn.microsoft.com/en-us/windows-server/administration/windows-commands/netsh-interface#interface-portproxy)
and
[`New-NetFirewallRule`](https://learn.microsoft.com/en-us/powershell/module/netsecurity/new-netfirewallrule).

## Development

Requires Go 1.26 or later and Make.

```sh
make build
./bin/lmu-archiver --help
make run ARGS=--help
./bin/lmu-archiver auth --lmu-url http://192.0.2.10:6937
make test
```

Replace the example IP with your LMU PC's address. On success, `auth` prints only
`Authentication succeeded.` and discards the in-memory session on exit. It does
not print or save credentials for a later invocation. LMU must be running; set
`--lmu-url` to the REST API address it listens on.

Tests use local HTTP servers and do not contact LMU or RaceOS.

## Container

```sh
docker build -t lmu-archiver .
docker run --rm lmu-archiver --help
```

The runtime image is `gcr.io/distroless/static-debian13:nonroot`. It runs as a
non-root user. To authenticate against LMU on another PC, pass its reachable URL:

```sh
docker run --rm lmu-archiver auth --lmu-url http://192.0.2.10:6937
```

Inside a container, `127.0.0.1` refers to the container, not the LMU PC.
