# lmu-archiver

A command-line tool for Le Mans Ultimate, intended to retrieve and archive your
online racing data under your own control.

## Status

The Go CLI authenticates with RaceOS (`raceos.gg`), imports existing results JSON
into a local SQLite archive, and records current DR/SR observations from the
authenticated profile. JSON queries expose latest races, observation history,
and read-time reference ratings. Automated results API retrieval is not included.
Minimal local mocks support authentication without either service.

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

## Considerate API usage

Keeping the load on LMU and RaceOS as low as practical takes priority over
retrieval speed. The aim is to avoid burdening the services we depend on, not to
maximize request throughput.

The CLI shares one HTTP client across both services to prevent concentrated
access from concurrent callers, after failures, or when resuming after idle time.
As a concrete safeguard, each transport attempt is followed by at least one second
before the next request, measured from response headers or a communication error.
Idle time does not accumulate permission for a burst, and failed attempts also
consume the cooldown. HTTP errors, including 429, are not automatically retried.

Waiting is cancelable and counts toward the client's 20-second request timeout.
The existing 1.2-second authentication-specific wait remains. New API clients must
receive the same HTTP client to participate in this protection. Response body
consumption does not hold the shared gate.

Separate CLI processes, underlying transport retries, and server arrival timing
are outside this guarantee. This safeguard does not eliminate service load or
guarantee compliance with service-side usage limits.

## Project layout

```text
cmd/lmu-archiver/     CLI entry point
cmd/mock-apis/        Local LMU and RaceOS mock servers
internal/auth/       LMU ticket acquisition and RaceOS authentication
internal/httpclient/ Shared HTTP client and API request cooldown
internal/mockapi/    Service-specific mock HTTP handlers
internal/ingest/     Own-result parsing and credential-free profile extraction
internal/storage/    SQLite persistence and temporal queries
db/migrations/      Embedded goose SQL migrations
bin/                 Local build output (ignored except for .gitkeep)
go.mod               Go module definition
Makefile             Local build, run, mock, and test commands
Dockerfile           Multi-stage CLI container build
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

## SQLite archive

The archive uses [goose](https://github.com/pressly/goose) SQL migrations embedded
in the CLI and a pure-Go SQLite driver; no external goose or SQLite executable
is required. Every archive command creates/migrates `data/archive.sqlite3` by
default. Override it with `--db`. Local `data/` is ignored by Git.

```sh
# Create or upgrade an archive explicitly.
./bin/lmu-archiver archive migrate --db data/archive.sqlite3

# Import an existing GET /api/v1/results JSON array for your exact Steam ID.
# The input file remains separate; it is not copied into the database.
./bin/lmu-archiver archive import-results \
  --input /path/to/results.json --steam-id YOUR_STEAM_ID

# Record the current profile and its exact DR/SR, without saving credentials.
# The Steam ID is resolved from the authenticated profile.
./bin/lmu-archiver archive record-rating \
  --lmu-url http://192.0.2.10:6937 --raw-dir data/raw

# Latest 10 events with your race results, including your other sessions.
./bin/lmu-archiver archive races --steam-id YOUR_STEAM_ID --limit 10

# Rating observations for graphing: inclusive start, exclusive end.
./bin/lmu-archiver archive ratings --steam-id YOUR_STEAM_ID \
  --from 2026-10-01T00:00:00Z --to 2026-11-01T00:00:00Z
```

`races` and `ratings` print JSON. `ratings` defaults to the last 30 days.
Import validates the entire file and commits it atomically; any event missing
your uniquely identified driver or all of your session results fails the import.
Reimport replaces your sessions without duplicating events or deleting another
tracked player's results. Other participants remain in the original JSON.

Each race's `referenceRating` is the latest observation for the same player at
or before the API event timestamp, or `null` if none exists. It is resolved on
read, never copied into an event, and is **not a confirmed post-race rating**.
Keep `observedAt` visible when presenting this reference. Exact historical
per-race changes cannot be recovered from result arrows.

`record-rating` saves only explicitly selected, credential-free profile fields
in a separate JSON file (not the raw authentication response), then stores the
observation and file path. New database/profile files use mode `0600`, and new
parent directories use `0700`. Existing permissions are not changed. If database
saving fails after the profile is written, that profile file remains available.
Protect results JSON too: it contains other drivers' names and Steam IDs.

See the [SQLite reference](docs/sqlite.md) for tables, constraints, indexes,
query examples, and migration/backup rules.

## Development

Requires Go 1.26 or later and Make.

```sh
make build
./bin/lmu-archiver --help
make run ARGS=--help
./bin/lmu-archiver auth --lmu-url http://192.0.2.10:6937
make test
make test-e2e
```

Replace the example IP with your LMU PC's address. On success, `auth` prints only
`Authentication succeeded.` and discards the in-memory session on exit. It does
not print or save credentials for a later invocation. For real authentication,
LMU must be running; set `--lmu-url` to the REST API address it listens on.
`--raceos-url` defaults to `https://raceos.gg`. Override it only with a trusted
server: the CLI sends the Steam session ticket to that origin.

Tests use local HTTP servers and do not contact LMU or RaceOS.

### Pull request checks

GitHub Actions runs two independent jobs on every pull request, including updates:

- **Go tests** runs `make test` (`go test ./...`).
- **Mock E2E** runs `make test-e2e`
  (`go test -tags=e2e -count=1 -timeout=2m -v ./e2e`).

Both jobs use `ubuntu-slim`; third-party actions are pinned to full commit SHAs.

The E2E suite builds and executes the real CLI against local HTTP servers on
ephemeral loopback ports, reusing `internal/mockapi` for LMU and RaceOS. It checks
successful authentication, LMU failure, RaceOS rejection, and malformed RaceOS
responses, including exit status, credential-safe output, and no RaceOS request
after LMU failure. Production HTTP cooldowns remain enabled.

No running game, Steam account, external API access, or secrets are required.
The `e2e` build tag keeps these subprocess scenarios separate from ordinary tests.

### Local API mocks

The mocks cover only the authentication endpoints currently used by the CLI.
They use fixed fake credentials, make no upstream requests, and do not emulate
Steam, token expiry, or data retrieval.

Start both servers in one terminal:

```sh
make mock
# LMU mock: http://127.0.0.1:6937
# RaceOS mock: http://127.0.0.1:8081
```

In another terminal, build and run the real CLI against **both** mock origins:

```sh
make build
./bin/lmu-archiver auth \
  --lmu-url http://127.0.0.1:6937 \
  --raceos-url http://127.0.0.1:8081
# Authentication succeeded.
```

`make build` also produces `bin/mock-apis`, which can be run directly.
Stop the servers with Ctrl-C. Both listeners default to loopback; these are
development servers without authentication or TLS, not services to expose.
To avoid a port conflict with a running LMU instance, change the listeners and
pass the matching origins to the CLI:

```sh
make mock ARGS='--lmu-listen 127.0.0.1:16937 --raceos-listen 127.0.0.1:18081'
```

Supported contracts:

| Service | Request | Response |
| --- | --- | --- |
| LMU | `GET /rest/profile/getAuthSessionTicket` | `{"authSessionTicket":"mock-steam-ticket"}` |
| RaceOS | `POST /authenticate` with JSON `{"token":"mock-steam-ticket","game":"lmu","platform":"steam"}` | `{"accessToken":"mock-access-token"}` |

RaceOS returns 400 for malformed or trailing JSON and 401 for incorrect or missing
credentials. Unknown JSON fields are ignored. Unimplemented paths return 404;
unsupported methods return 405 (standard `ServeMux` GET routes also accept HEAD).
The normal CLI request cooldown and authentication wait remain enabled.

To extend the mocks, register method-qualified routes in `internal/mockapi/lmu.go`
or `internal/mockapi/raceos.go`. Each constructor returns a separate
`*http.ServeMux`, so callers can also add routes before serving it. Keep new
endpoints service-specific and use only fake data; no generic fixture engine or
complete API emulator is required.

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
