# Workflow

[日本語](workflow.ja.md)

## Download and ingest

`lmu-archiver sync` performs one complete operation:

1. Obtain a Steam session ticket from running LMU and authenticate with RaceOS.
2. Extract the current DR/SR and Steam identity from the authentication profile.
3. Fetch the complete result history using the same rate-limited HTTP client.
4. Save result JSON, an allowlisted credential-free profile, and a manifest
   containing the original observation timestamp and stable capture ID.
5. Import the player's event/session results and save DR/SR if its values changed.

Tickets and access tokens are never persisted. Default event types are
`daily,specialevent`. Empty/null pages end pagination; repeated pages and HTTP
errors fail the operation. Failed acquisition leaves partial files for inspection
but does not import into the database. Database failures retain complete captures
for offline replay. No automatic retries are performed.

The first successful import registers the SQLite file's sole Steam account.
Later imports with a different Steam ID fail without changing existing archive
data. Use a separate `--db` file for each account.

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

## Duplicate prevention

| Purpose | Mechanism | Scope / behavior |
| --- | --- | --- |
| Avoid repeating a capture's rating processing | `capture_observations`, keyed by `(capture_id, player_id)` | Uses the manifest's capture ID, not a file-content hash. Result rows are still reimported. |
| Avoid storing unchanged DR/SR | Compare all eight values with the player's latest stored `rating_snapshots` row at or before the capture's observation time | Insert only on the first observation or a value change. A return to an earlier value (`A → B → A`) is still recorded. |
| Avoid duplicate event/result rows in SQLite | Primary keys, event/participation upserts, and replacement of session results | Reimport updates the archive owner's results without duplicating events. |
| Avoid downloading an already stored event again | Not performed | Every online sync fetches the complete result history so that historical corrections can be imported. |

## Queries and HTTP API

`races` and `ratings` read the archive as tables or `--json`. `serve` exposes
those reads, plus per-track statistics and filter values, through a read-only
HTTP API. Reads never download, create a database, or apply migrations. The
account comes only from the database; there is no environment/config/read-command
selector. An empty database requires `sync` first; legacy databases with multiple
Steam users are rejected.
Requests cannot change the account; the frontend needs no player selector.
See the [API reference](lmu-archiver-api.yaml) for response types and examples.

The HTTP server has no authentication or TLS. Keep its default loopback binding
and do not expose it to untrusted networks.

Reference ratings are resolved on read from observations at or before the event
timestamp, not asserted post-race values. Raw results remain separate files; only
the tracked player's data is normalized in SQLite.

See the [README](../README.md) for commands/configuration and
[SQLite reference](sqlite.md) for storage details.
