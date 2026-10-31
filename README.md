# lmu-archiver

A command-line tool for Le Mans Ultimate, intended to retrieve and archive your
online racing data under your own control.

## Status

**Work in progress.** The project currently contains only the Go CLI entry point
and build configuration. The CLI prints help; authentication, data retrieval, and
archiving are not implemented yet.

## Goals and data ownership

Unlike companion services that sync racing data to their own hosted accounts,
lmu-archiver aims to let you keep ownership and control of your data: where it is
stored, how it is used, and whether it is shared.

The goal is local archiving without a required third-party dashboard, analytics,
or cloud synchronization. This does not remove the need to contact the game's
online services to retrieve your data.

## Authentication reference

The planned authentication flow is informed by
[LMU Central](https://github.com/MarteauPiqueurdu11/LMU-Central), especially its
[RaceOS client](https://github.com/MarteauPiqueurdu11/LMU-Central/blob/main/lmu_central/core/raceos.py)
and
[technical notes](https://github.com/MarteauPiqueurdu11/LMU-Central/blob/main/docs/notes-techniques.md).
These are implementation references, not runtime dependencies. Authentication is
not implemented in this repository yet.

This project is independent and is not affiliated with Le Mans Ultimate,
Studio 397, Motorsport Games, or the referenced tools.

## Project layout

```text
cmd/lmu-archiver/   CLI entry point
internal/          Reserved for application-internal packages
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
make test
```

There are no tests yet. The current CLI does not make network requests.

## Container

```sh
docker build -t lmu-archiver .
docker run --rm lmu-archiver --help
```

The runtime image is `gcr.io/distroless/static-debian13:nonroot`. It runs as a
non-root user and currently only prints CLI help.
