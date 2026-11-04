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
| Avoid duplicate event/result rows in SQLite | Primary keys, event/participation upserts, and replacement of the tracked player's session results | Reimport updates existing results without duplicating events or deleting another tracked player's results. |
| Avoid downloading an already stored event again | Not performed | Every online sync fetches the complete result history so that historical corrections can be imported. |

## Queries and HTTP API

`races` and `ratings` read the archive as tables or `--json`. `serve` exposes the
same query operations through a read-only HTTP API. Reads never download, create
a database, or apply migrations. The HTTP API serves one account fixed at server
startup, selected by `serve --steam-id`, environment `STEAM_ID`, config `steamId`,
or the only player in SQLite. An explicitly blank `STEAM_ID` clears config
`steamId` rather than falling through to it. Export a trusted `.env` in the shell
before starting `serve`; it is not loaded automatically. Without a selected ID,
zero or multiple players prevent startup.
Requests cannot change this account; the frontend needs no player selector.
See the [OpenAPI contract](openapi.yaml) for response schemas and query parameters.

The HTTP server has no authentication or TLS. Keep its default loopback binding
and do not expose it to untrusted networks.

Reference ratings are resolved on read from observations at or before the event
timestamp, not asserted post-race values. Raw results remain separate files; only
the tracked player's data is normalized in SQLite.

See the [README](../README.md) for commands/configuration and
[SQLite reference](sqlite.md) for storage details.
