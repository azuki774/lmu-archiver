# Workflow

[日本語](workflow.ja.md)

## Download and ingest

`lmu-archiver sync` performs one complete operation:

1. Obtain a Steam session ticket from running LMU and authenticate with RaceOS.
2. Extract the current DR/SR and Steam identity from the authentication profile.
3. Fetch paginated results using the same rate-limited HTTP client.
4. Save result JSON, an allowlisted credential-free profile, and a manifest
   containing the original observation timestamp and stable capture ID.
5. Commit the player's event/session results and rating observation together in
   one SQLite transaction.

Tickets and access tokens are never persisted. Default event types are
`daily,specialevent`. Empty/null pages end pagination; repeated pages and HTTP
errors fail the operation. Failed acquisition leaves partial files for inspection
but does not import into the database. Database failures retain complete captures
for offline replay. No automatic retries are performed.

## Offline replay

```sh
lmu-archiver sync --skip-download --input /path/to/capture
lmu-archiver sync --skip-download --input /path/to/results.json --steam-id YOUR_STEAM_ID
```

Replay performs no authentication or network access. Complete captures restore
results and ratings, preserving the original observation time and recording each
capture's observation at most once per player. A legacy results-only file imports
only results; it cannot reconstruct DR/SR. Keep the manifest and source files
immutable and together.

## Queries and HTTP API

`races` and `ratings` read the archive as tables or `--json`. `serve` exposes the
same query operations through a read-only HTTP API. Reads never download, create
a database, or apply migrations. A single-player archive needs no Steam ID flag;
multiple players require an explicit argument or configuration.

The HTTP server has no authentication or TLS. Keep its default loopback binding
and do not expose it to untrusted networks.

Reference ratings are resolved on read from observations at or before the event
timestamp, not asserted post-race values. Raw results remain separate files; only
the tracked player's data is normalized in SQLite.

See the [README](../README.md) for commands/configuration and
[SQLite reference](sqlite.md) for storage details.
