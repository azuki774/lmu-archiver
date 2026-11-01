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

See the [workflow](docs/workflow.md) / [ワークフロー](docs/workflow.ja.md) for the
protocol, prerequisites, credential handling, and planned data-processing flow.

This project is independent and is not affiliated with Le Mans Ultimate,
Studio 397, Motorsport Games, or the referenced tools.

## Project layout

```text
cmd/lmu-archiver/   CLI entry point
internal/auth/     LMU ticket acquisition and RaceOS authentication
bin/               Local build output (ignored except for .gitkeep)
go.mod             Go module definition
Makefile           Local build, run, and test commands
Dockerfile         Multi-stage container build
```

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
not print or save credentials for a later invocation. See the workflow for
connection prerequisites and the port to use.

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
