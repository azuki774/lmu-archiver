# lmu-archiver

A command-line tool for Le Mans Ultimate, intended to retrieve and archive your
online racing data under your own control.

## Status

`sync` authenticates with RaceOS (`raceos.gg`), downloads all race results and the
current DR/SR profile, saves credential-free JSON captures, and imports results
and changed ratings into SQLite. `sync --skip-download` replays saved inputs without network access.
`serve` exposes the archive as a read-only JSON API for future frontends.
Local mocks support the complete synchronization flow without either service.

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
internal/archive/   Download and offline capture ingestion
internal/query/     Shared CLI and HTTP query operations
internal/api/       Read-only HTTP API
internal/storage/    SQLite persistence and temporal queries
db/migrations/      Embedded goose SQL migrations
testdata/mock-api/   Synthetic JSON response templates for mocks and tests
bin/                 Local build output (ignored except for .gitkeep)
go.mod               Go module definition
Makefile             Local build, run, mock, and test commands
Dockerfile           Multi-stage CLI container build
Dockerfile.mock      Multi-stage Alpine mock API container build
```

## Local environment

Copy the reference configuration and adjust the LMU and RaceOS origins for your
environment:

```sh
cp .env.sample .env
```

The sample uses the local Windows API address and production RaceOS:

```dotenv
LMU_INT_API="http://127.0.0.1:6937/"
RACEOS_URL="https://raceos.gg"
```

The CLI reads exported `LMU_INT_API` and `RACEOS_URL` environment variables.
`.env` is ignored by Git and is not loaded automatically; `.env.sample` is the
shared reference. After building, export a trusted local file in a POSIX shell:

```sh
set -a
. ./.env
set +a
./bin/lmu-archiver auth check
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

The archive uses embedded [goose](https://github.com/pressly/goose) migrations
and a pure-Go SQLite driver; no external goose or SQLite executable is required.
`sync` and `db migrate` create/upgrade `./data/archive.sqlite3`. `races`, `ratings`,
and `serve` open it read-only: they never create files or apply migrations.
Missing or outdated databases require `sync` or `db migrate` first.

Paths are relative to the process's working directory, not the executable.
New captures default to `./data/raw`; override paths with `--db` and `--raw-dir`.
The repository-root `data/` directory is ignored by Git.

```sh
# With LMU running: authenticate, download results + current DR/SR, save and ingest.
./bin/lmu-archiver sync --lmu-url http://192.0.2.10:6937

# Replay a complete capture. No LMU, authentication, or network connection needed.
./bin/lmu-archiver sync --skip-download --input /path/to/capture-directory

# Import a pre-existing results-only JSON array or null page.
# Without a capture manifest, identity must be supplied and no rating is invented.
./bin/lmu-archiver sync --skip-download --input /path/to/results.json \
  --steam-id YOUR_STEAM_ID

# Player selection is automatic when the archive contains exactly one player.
./bin/lmu-archiver races --limit 10
./bin/lmu-archiver ratings --from 2026-10-01 --to 2026-11-01

# Machine-readable output.
./bin/lmu-archiver races --json
./bin/lmu-archiver ratings --json

# Explicit schema administration.
./bin/lmu-archiver db migrate --db data/archive.sqlite3
```

`races` and `ratings` show tables by default; `--json` returns JSON arrays.
`sync --json` prints a summary with `steamId`, `capturePath`, `events`, and
`ratingRecorded` (whether a new observation was inserted). `events` counts events
downloaded/imported in this capture, not all events in the archive. Progress/errors use
stderr, separate from stdout. Exit codes: 0 success, 1 operational failure,
2 invalid command arguments or configuration.

Rating ranges include `from` and exclude `to`. Dates use UTC; RFC3339 timestamps
are also accepted. The default range is the last 30 days. Specify `--steam-id`
or configure `steamId` when tracking multiple players.

Synchronization requests `daily,specialevent` by default, using 20 results per
page. `--event-types` overrides the comma-separated list. `championship` can be
explicitly requested, but its API support is unverified: an empty response does
not prove support. Repeated pages fail instead of looping indefinitely.
Duplicate event IDs are imported once, using the first occurrence.

### Result refresh and rating changes

Every online sync fetches all pages until an empty array or null response,
including previously stored events. Reimporting those events reflects historical
corrections on the next successful sync if the service returns them.
Events missing from API responses are not deleted from the archive.

DR/SR snapshots are saved on the first observation and when any of these values
differs from the same player's preceding stored observation:

- DR ELO, rank, tier, and progress.
- SR rating, rank, tier, and progress.

Comparison is exact, without rounding or tolerance. Source-file paths and other
metadata do not count as rating changes. A return to an older value (`A → B → A`)
is still a change, not a duplicate. Older captures are compared at their original
observation time, never against future ratings.

The database therefore records observed rating changes, not every successful
check. An unchanged value does not add a snapshot or advance its `observedAt`.
`referenceRating.observedAt` remains the time of the stored change observation,
not the last time the value was checked. Existing snapshots are not removed.
DR/SR still comes from each authentication response; this reduces DB inserts,
not API requests.

All results are validated before data is committed. An event missing your
uniquely identified driver or all of your session results fails the import.
Events, the capture marker, and any changed rating are committed in one transaction.
Reimport replaces your sessions without duplicating events or deleting another
tracked player's results. Other participants remain in the original JSON.

Each race's `referenceRating` is the latest observation for the same player at
or before the API event timestamp, or `null` if none exists. It is resolved on
read, never copied into an event, and is **not a confirmed post-race rating**.
Keep `observedAt` visible when presenting this reference. Exact historical
per-race changes cannot be recovered from result arrows.

### Replayable captures

Each online sync creates a private directory containing:

- `manifest.json`: format version 1, stable `captureId`, `steamId`, original
  `observedAt`, requested `eventTypes`, and relative `results`/`profile` paths.
- `results.json`: combined, deduplicated result events.
- `profile.json`: allowlisted identity and exact DR/SR fields, not the raw
  authentication response.
- `<event-type>-page-NNNN.json`: individual responses for provenance.

The manifest is written after acquisition finishes. Failed downloads retain
partial files for inspection, report their location, and do not import data.
A DB failure retains the complete capture; retry it with `--skip-download`.
There are no automatic retries or interrupted-download resume behavior.

Replay preserves the original rating timestamp and processes that capture's
rating at most once per player, even when unchanged values are skipped.
Results are still reimported. Keep captures immutable and retain their IDs when
moving or backing them up. Relative manifest paths allow replay into a new
database after moving the capture directory.

New database/JSON files use mode `0600`; new directories use `0700`.
Existing permissions are not changed. Tickets and access tokens are never
persisted. Protect captures too: results contain other drivers' names and Steam IDs.

See the [SQLite reference](docs/sqlite.md) for tables, constraints, indexes,
query examples, and migration/backup rules.

## Configuration

An optional JSON configuration is loaded from
`$XDG_CONFIG_HOME/lmu-archiver/config.json` (normally
`~/.config/lmu-archiver/config.json` on Linux). Use `--config PATH` before the
command or among its flags to select another file. For API origins, precedence
is **explicit flags > environment > JSON config > built-in defaults**:
`LMU_INT_API` sets the LMU origin and `RACEOS_URL` sets the RaceOS origin used
for both authentication and results. Other settings use flags > config > defaults.
An explicitly empty or invalid origin fails online commands instead of falling
back to production; offline imports and read-only queries do not validate or
contact these origins. Connection failures never fall back to another origin.
Unknown config keys, invalid JSON, and explicitly selected missing files are
errors. Paths remain relative to the process's working directory, not the config file.

```json
{
  "db": "/path/to/archive.sqlite3",
  "rawDir": "/path/to/raw",
  "lmuUrl": "http://192.0.2.10:6937",
  "raceosUrl": "https://raceos.gg",
  "listen": "127.0.0.1:8080",
  "eventTypes": ["daily", "specialevent"]
}
```

Optionally add `"steamId": "YOUR_STEAM_ID"` for offline results files or
multi-player archives. Online sync verifies it against the authenticated profile.
Configuration contains no session credentials; `.env` is not loaded automatically.

## Read-only HTTP API

```sh
./bin/lmu-archiver serve --listen 127.0.0.1:8080
curl 'http://127.0.0.1:8080/api/v1/races?limit=10'
curl 'http://127.0.0.1:8080/api/v1/ratings?from=2026-10-01&to=2026-11-01'
```

- `GET /api/v1/races`: optional `steam-id`, positive `limit` (default 10).
- `GET /api/v1/ratings`: optional `steam-id`, `from`, `to`, with the same
  defaults and time boundaries as the CLI.
- Successful responses are the same JSON arrays as CLI `--json`. Invalid query
  inputs return 400, unknown routes 404, unsupported methods 405, and query
  failures 500. Errors use `{"error":"..."}` without database internals.
- `serve` does not authenticate, synchronize, migrate, or make upstream requests.
  A separate `sync` process can update the database while it is running.
- Ctrl-C/SIGTERM shuts the server down gracefully.

The default listener is loopback. **There is no HTTP authentication, TLS, or
CORS configuration.** Do not expose it to an untrusted network; access to this
API grants access to your archive, including identity and source-file paths.
No frontend or HTTP synchronization endpoint is included.

## Command migration

The previous `archive` command group has been removed. Use `sync` instead of
`archive record-rating`, `sync --skip-download --input ...` instead of
`archive import-results`, and top-level `races` / `ratings` for queries.
`archive migrate` is now `db migrate`; the authentication diagnostic is
`auth check`. There are no legacy command aliases.


## Development

Requires Go 1.26 or later and Make.

```sh
make build
./bin/lmu-archiver --help
make run ARGS=--help
./bin/lmu-archiver auth check --lmu-url http://192.0.2.10:6937
make test
make test-e2e
```

Replace the example IP with your LMU PC's address. On success, `auth check` prints
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

Each E2E scenario copies `testdata/mock-api/` into its own temporary directory
before starting its mock servers. Authentication scenarios execute the real CLI
against ephemeral loopback HTTP servers and check success, LMU failure, RaceOS
rejection, malformed responses, exit status, credential-safe output, and no
RaceOS request after LMU failure.

The synchronization scenario builds both executables, starts the standalone mock
outside the repository, and supplies its origins through environment variables.
It checks both results pages, exact saved values, a new race and an existing-event
correction after editing the response files, updated DR/SR, and replay without
duplicating observations or events. Production HTTP cooldowns remain enabled.

No running game, Steam account, external API access, or secrets are required.
The `e2e` build tag keeps these subprocess scenarios separate from ordinary tests.

### Local API mocks

The standalone `mock-apis` process serves external JSON files. It needs no game,
Steam account, CLI process, or upstream connection. Credential checks use the fixed
fake ticket/token shown below; Steam and token expiry are not emulated.

Prepare an editable copy of the committed, entirely synthetic response templates:

```sh
make mock-init
```

This copies `testdata/mock-api/` into the ignored `data/mock-api/` directory.
Running it again overwrites the baseline files; additional files are not removed.
Keep edits in the working copy rather than changing shared test templates.

Start both services in one terminal:

```sh
make mock
# LMU mock: http://127.0.0.1:6937
# RaceOS mock: http://127.0.0.1:8081
```

In another terminal:

```sh
make build
LMU_INT_API=http://127.0.0.1:6937 \
RACEOS_URL=http://127.0.0.1:8081 \
./bin/lmu-archiver sync \
  --db ./data/dev/archive.sqlite3 \
  --raw-dir ./data/dev/raw
```

Separate dev archive paths prevent dummy races from entering a real archive.
`make build` also produces `bin/mock-apis`, runnable independently from any
directory when given the response directory explicitly:

```sh
/path/to/bin/mock-apis --data-dir /absolute/path/to/mock-data \
  --lmu-listen 127.0.0.1:16937 \
  --raceos-listen 127.0.0.1:18081
```

Set `LMU_INT_API=http://127.0.0.1:16937` and
`RACEOS_URL=http://127.0.0.1:18081` for those listeners. The alternative ports avoid
conflicting with a running LMU instance. `make mock ARGS='--data-dir ...'` also
accepts these options. Stop with Ctrl-C. Both listeners default to loopback;
there is no TLS or production authentication. Do not expose them to untrusted networks.

#### Response files and updates

The default `--data-dir` is `data/mock-api`, relative to the working directory:

```text
lmu/auth-session-ticket.json
raceos/authenticate.json
raceos/results/daily/page-1.json
raceos/results/daily/page-2.json
raceos/results/daily/page-3.json
raceos/results/specialevent/page-1.json
raceos/results/championship/page-1.json
```

Daily templates contain 20 events on page 1 and 5 on page 2, newest first.
Page 3 contains literal `null` to end pagination; the other event types start
with `null`. The baseline includes fictional opponents, qualifying/practice
sessions, DNF and missing optional values. Every identity is a dummy.

The handler reads the matching file on **every request**, without caching.
To simulate updates, finish the current sync, edit the working JSON, then sync
again. For example, change `player.driverRank.elo` or `player.safetyRank.rating`
in `raceos/authenticate.json`. For a new event, use a new dummy `eventId`, keep
the tracked `steamId` consistent in `drivers` and session results, and split the
newest-first events into pages of at most 20. Keep an explicit `null` or `[]`
page after the final data page. Retain an event's ID when correcting its results.
Keep the fixed fake credentials unchanged.

There are no automatic updates, snapshot locks, or multi-file atomic swaps.
Do not edit during synchronization: pages and profile could come from different
versions. Missing/unreadable response files or invalid JSON return a safe JSON
500 error, not an empty successful response. The requested page maps directly to
`page-N.json`; a missing page is not an implicit terminator.

Supported contracts:

| Service | Request | Response |
| --- | --- | --- |
| LMU | `GET /rest/profile/getAuthSessionTicket` | `{"authSessionTicket":"mock-steam-ticket"}` |
| RaceOS | `POST /authenticate` with JSON `{"token":"mock-steam-ticket","game":"lmu","platform":"steam"}` | `accessToken` and fake player identity/DR/SR |
| RaceOS | `GET /api/v1/results?eventType=daily&page=1&take=20` with `Game-Authorization: Bearer mock-access-token` | Matching `raceos/results/daily/page-N.json`: 20 events, 5 events, then null |

RaceOS returns 400 for malformed or trailing JSON and 401 for incorrect or missing
credentials. Unknown JSON fields are ignored. Unimplemented paths return 404;
unsupported methods return 405 (standard `ServeMux` GET routes also accept HEAD).
The normal CLI request cooldown and authentication wait remain enabled.

To extend the mocks, register method-qualified routes in `internal/mockapi/lmu.go`
or `internal/mockapi/raceos.go` and add synthetic response files. Each constructor
accepts a data directory and returns a separate `*http.ServeMux`. Keep endpoints
service-specific; no generic fixture engine or complete API emulator is required.

## Container

```sh
docker build -t lmu-archiver .
docker run --rm lmu-archiver --help
```

The runtime image is `gcr.io/distroless/static-debian13:nonroot`. It runs as a
non-root user. To authenticate against LMU on another PC, pass its reachable URL:

```sh
docker run --rm lmu-archiver auth check --lmu-url http://192.0.2.10:6937
```

Inside a container, `127.0.0.1` refers to the container, not the LMU PC.

### Published images

GitHub Actions publishes `ghcr.io/azuki774/lmu-archiver` and
`ghcr.io/azuki774/lmu-archiver-mock` with matching tags:

- A push to `master` uses the first seven characters of the commit SHA, for
  example `2e10bb1`.
- A pushed Git tag uses the exact tag name and also updates `latest` for both
  images. This includes pre-release tags and tags on older commits.
- A manual workflow run on `master` can publish a specified full commit SHA
  reachable from `master`, using the first seven characters as the image tag.

GitHub uses the workflow at the tagged commit. Tags on commits that do not yet
contain this workflow's tag trigger will not publish images.

Git tags must also be valid Docker tags: up to 128 ASCII letters, digits,
underscores, periods, or hyphens, starting with a letter, digit, or underscore.
For example, to publish `1.0.2` and `latest`:

```sh
git tag 1.0.2
git push origin 1.0.2
```

After the publish workflow succeeds:

```sh
docker pull ghcr.io/azuki774/lmu-archiver:1.0.2
docker pull ghcr.io/azuki774/lmu-archiver-mock:1.0.2
```

### Mock API container

The separate Alpine image runs both mock servers in one process, with the
synthetic JSON templates included under `/data/mock-api`:

```sh
docker build -f Dockerfile.mock -t lmu-archiver-mock .
docker run --rm --name lmu-mock \
  -p 127.0.0.1:6937:6937 -p 127.0.0.1:8081:8081 \
  lmu-archiver-mock
```

The LMU and RaceOS origins on the host are respectively
`http://127.0.0.1:6937` and `http://127.0.0.1:8081`. Point **both**
`LMU_INT_API` and `RACEOS_URL` at them when using the CLI. Both servers listen
on all container interfaces; the loopback port mappings above restrict host
access. These mocks have no TLS or production authentication: use only trusted
container networks and do not expose them to untrusted clients.

To persist and edit responses on the host, first run `make mock-init` as described
above, then use this alternative to the `docker run` command:

```sh
docker run --rm --name lmu-mock \
  -p 127.0.0.1:6937:6937 -p 127.0.0.1:8081:8081 \
  --mount "type=bind,src=$(pwd)/data/mock-api,dst=/data/mock-api,readonly" \
  lmu-archiver-mock
```

The bind mount replaces the included templates, so it must contain the complete
response tree and be readable by UID/GID `65532:65532`. Changes to its files are
visible on the next request; do not edit them during a sync. Without a mount,
edits inside the container are lost when it is removed. Alpine supplies `/bin/sh`
for inspecting or changing the included files. Append `--data-dir`,
`--lmu-listen`, or `--raceos-listen` after the image name to override the
container defaults.
