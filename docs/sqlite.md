# SQLite

This document describes the archive's SQLite schema, constraints, indexes, and
queries. Command-line usage is in the [README](../README.md#sqlite-archive).
The schema is defined by the [goose migrations](../db/migrations/);
the application queries are implemented in [storage](../internal/storage/store.go).

## Database configuration

- Default file: `./data/archive.sqlite3`; commands accept `--db`.
  Capture files default to `./data/raw`, configurable with `sync --raw-dir`.
  Relative paths are resolved against the process's current working directory,
  not the executable's directory. Neither default is hard-coded to `/data`.
- The repository's `/data/` ignore rule excludes its root-level `data/`
  directory. The leading slash anchors the Git pattern to the repository root;
  it does not select an absolute storage path.
- The working directory must be writable by the process user. In a container,
  configure a writable working directory or override the paths to a writable
  mount; a relative path does not itself grant filesystem permissions.
- Driver: `modernc.org/sqlite`, through Go's `database/sql`; no external SQLite
  executable is required by the application.
- All six application tables use `STRICT` typing, requiring SQLite 3.37.0 or later for
  external tools that open the database.
- Each application connection enables `PRAGMA foreign_keys = ON` and sets
  `PRAGMA busy_timeout = 5000`. The application pool allows one open connection.
  External writers must enable foreign-key enforcement on their own connections.
- New database files are created with mode `0600`, and new parent directories
  with `0700`. Existing permissions are not changed.

## Storage conventions

| Value | SQLite representation |
| --- | --- |
| Internal player and snapshot IDs | `INTEGER PRIMARY KEY`, using SQLite's rowid |
| Steam, RaceOS, event, and series IDs | `TEXT`; not numbers or display names |
| Timestamps | `INTEGER`, microseconds since the Unix epoch in UTC |
| Lap and finish times | `INTEGER`, elapsed microseconds |
| Rank numbers and lap counts | `INTEGER`; missing result values are `NULL` |
| Rating values and progress | `REAL`, at SQLite floating-point precision |
| Boolean flags | `INTEGER`, constrained to `0` or `1` |
| Car and split numbers | `TEXT`, preserving leading zeros |
| Source JSON references | Absolute filesystem paths stored as `TEXT` |

`event_at` is the event's API timestamp, `imported_at` is the result import time,
and `observed_at` is when a rating was obtained. These timestamps are not
interchangeable. Timestamp precision below one microsecond is not retained.
The API event timestamp has not been verified as the actual race start or end.

Raw result JSON stays in separate files; it is not stored in a database column.
Profile files contain only allowlisted identity and rating fields, never the full
authentication response, Steam tickets, or access tokens. Paths are provenance
references, not foreign keys: SQLite does not check that the files exist or have
not changed. Moving a file does not update its stored path.

## Tables and relationships

| Table | Stored data | Purpose |
| --- | --- | --- |
| `players` | Steam ID, RaceOS player ID, and display name | Identify the archive's sole account by Steam ID rather than display name. |
| `events` | Event timestamp, title, type, track, split, and source-file path | Store shared event information; update existing event IDs when results are reimported. |
| `event_participations` | A player's name, car class, result-reported ranks, and adjustment indicators for an event | Link a tracked player to an event and keep player-specific information separate from shared event metadata. Adjustment indicators are not exact ELO/SR deltas. |
| `session_results` | Positions, grid positions, laps, lap/finish times, and finish status | Store the tracked player's performance in each practice, qualifying, warmup, or race block. |
| `rating_snapshots` | Observation timestamp and the eight exact DR/SR value fields | Store the first observation and subsequent value changes, independently of individual events. |
| `capture_observations` | Processed `(capture_id, player_id)` pairs | Prevent replaying the same capture's rating processing, including captures whose unchanged rating was skipped. This does not suppress result reimports or compare DR/SR values. |
| `goose_db_version` (goose-managed) | Applied migration versions | Track the database schema version; not an application-data table. |

```text
players ──< event_participations >── events
                 │
                 └──< session_results
players ──< rating_snapshots
players ──< capture_observations
```

An event contains shared information. A participation contains one tracked
player's event-level information. Session results belong to that participation.
Rating snapshots belong to a player independently of any event.

Each SQLite file belongs to one Steam account, registered by its first successful
import. Application write transactions reject a different Steam ID before
updating any player, event, result, rating, or capture marker. Use separate files
for different accounts. Existing multi-user files are rejected for imports and
account resolution; no rows are deleted automatically.
Other participants remain available in the separate source JSON.

### `players`

| Column | Type | Constraints / meaning |
| --- | --- | --- |
| `player_id` | INTEGER | Primary key; internal account identifier |
| `steam_id` | TEXT | NOT NULL, UNIQUE, nonblank; exact identity used for imports |
| `raceos_player_id` | TEXT | NOT NULL, default empty string; authenticated profile ID when known |
| `display_name` | TEXT | NOT NULL; account display name, not an identity key |

Names and RaceOS IDs can be updated. An incoming empty name or RaceOS ID does not
erase an existing value. Event-specific names are stored separately below.

### `events`

| Column(s) | Type | Constraints / meaning |
| --- | --- | --- |
| `event_id` | TEXT | Primary key, NOT NULL, nonblank; API event ID |
| `series_id` | TEXT | NOT NULL; API series ID |
| `event_at` | INTEGER | NOT NULL; API event timestamp |
| `title`, `event_type`, `tier` | TEXT | NOT NULL; event name and classifications |
| `track`, `track_scene` | TEXT | NOT NULL; API track identifiers |
| `split_no` | TEXT | NOT NULL; split identifier |
| `official`, `team_event` | INTEGER | NOT NULL; each constrained to `0` or `1` |
| `raw_path` | TEXT | NOT NULL; result JSON file used by the latest import |
| `imported_at` | INTEGER | NOT NULL; latest import timestamp |

There are no copied reference-rating values or snapshot IDs on this table.

### `event_participations`

Primary key: **`(event_id, player_id)`**. Both columns are NOT NULL and reference
`events(event_id)` and `players(player_id)`, respectively.

| Column(s) | Type | Constraints / meaning |
| --- | --- | --- |
| `display_name`, `car_class` | TEXT | NOT NULL; name and class reported for this participation |
| `dr_rank`, `sr_rank` | TEXT | Nullable rank names from the result response |
| `dr_tier`, `sr_tier` | INTEGER | Nullable rank tiers from the result response |
| `dr_progress`, `sr_progress` | REAL | Nullable progress values from the result response |
| `dr_adjustment`, `sr_adjustment` | INTEGER | Nullable; constrained to `-3` through `3` |
| `joker_played` | INTEGER | NOT NULL; constrained to `0` or `1` |

Adjustments are arrow indicators, **not numeric ELO or SR deltas**. Result-response
progress values are distinct from the precise profile observations in
`rating_snapshots`. The result API's zero ELO/rating fields are not imported as
rating observations.

### `session_results`

Primary key: **`(event_id, player_id, session_type, session_no, race_no)`**.
All key columns are NOT NULL. The composite foreign key `(event_id, player_id)`
references `event_participations(event_id, player_id)`.

| Column(s) | Type | Constraints / meaning |
| --- | --- | --- |
| `session_type` | TEXT | `practice`, `qualifying`, `warmup`, or `race` |
| `session_no` | TEXT | API `sessionNo`, retained as a string |
| `race_no` | INTEGER | Positive, one-based index into that session's `races` array |
| `position`, `class_position` | INTEGER | Nullable, positive finishing positions |
| `grid_position`, `class_grid_position` | INTEGER | Nullable, positive starting positions |
| `car_type`, `car_class`, `car_number`, `team_name` | TEXT | NOT NULL; session-specific vehicle information |
| `completed_laps` | INTEGER | Nullable, nonnegative lap count |
| `best_lap_us`, `finish_time_us` | INTEGER | Nullable, nonnegative elapsed microseconds |
| `finish_status` | TEXT | NOT NULL; API status such as `Finished Normally`, `DNF`, or `DQ` |

Empty numeric result strings become `NULL`, not zero. Decimal seconds are
converted without a binary floating-point intermediate. Nonzero precision below
one microsecond is rejected rather than rounded.

### `rating_snapshots`

| Column(s) | Type | Constraints / meaning |
| --- | --- | --- |
| `snapshot_id` | INTEGER | Primary key; orders observations sharing a timestamp |
| `player_id` | INTEGER | NOT NULL; foreign key to `players(player_id)` |
| `observed_at` | INTEGER | NOT NULL; time the profile was obtained |
| `source` | TEXT | NOT NULL, nonblank; `authenticate` for current CLI captures |
| `raw_path` | TEXT | NOT NULL; separate credential-free profile file |
| `dr_elo`, `sr_rating` | REAL | NOT NULL; observed DR/SR numeric values |
| `dr_rank`, `sr_rank` | TEXT | NOT NULL; observed rank names |
| `dr_tier`, `sr_tier` | INTEGER | NOT NULL; observed rank tiers |
| `dr_progress`, `sr_progress` | REAL | NOT NULL; observed rank progress |

An observation is inserted only when there is no preceding stored observation
for the player, or at least one of the eight DR/SR value fields differs.
Comparison uses the latest `(observed_at, snapshot_id)` at or before the incoming
observation timestamp, so historical replays do not compare against future data.
Numeric comparisons are exact; source/path/timestamp changes alone do not count.
A return to a previously seen value is saved if it differs from the preceding
value. Existing rows are not compacted or deleted.

Unchanged values leave the stored `observed_at` untouched. These rows describe
observed changes, not the last time a rating was checked. A present zero differs
from a missing field; sync rejects missing required identity/rating fields.
There is no event foreign key or per-race delta.

### `capture_observations`

Defined in the initial migration `00001_archive.sql`. Primary key:
**`(capture_id, player_id)`**. `capture_id` is nonblank TEXT from the capture's
manifest; `player_id` references `players(player_id)`.

The marker is inserted in the same transaction as results and any changed
rating, including captures whose unchanged rating needs no snapshot.
An existing marker suppresses only rating processing; result rows are still
reimported. Capture IDs must remain stable when backing up or relocating captures.

## Indexes

Primary keys and the Steam ID UNIQUE constraint provide their own lookup
structures; the integer primary keys use the rowid. The migration adds:

| Index | Columns, in order | Access path |
| --- | --- | --- |
| `idx_events_time` | `events(event_at, event_id)` | Event timestamp ordering with a deterministic tie-breaker |
| `idx_participations_player` | `event_participations(player_id, event_id)` | A player's participation lookup; also covers the child player foreign key |
| `idx_rating_snapshots_player_time` | `rating_snapshots(player_id, observed_at)` | Per-player time ranges and latest-prior observation lookup |

The session primary key begins with `(event_id, player_id)`, covering both
participation joins and its child foreign key. No duplicate index on those
columns is needed. A descending scan can use an ascending index. The snapshot
index also contains the rowid, which is `snapshot_id`, as its tie-breaker.

These indexes do not guarantee that every join avoids sorting. Use
`EXPLAIN QUERY PLAN` with representative data before adding indexes for track,
vehicle, or aggregate queries. A full scan is not inherently wrong for an
all-history aggregation.

## Query examples

The examples use SQLite named parameters. Bind `:steam_id` as TEXT, `:limit` as a
positive integer, and timestamp parameters as integer Unix microseconds, not
ISO-formatted text. They illustrate the supported reads without changing data.

### Latest N races with the tracked player's information

Select N **events with that player's race results**, then join their sessions.
A practice/qualifying/warmup row must not consume a place in the event limit.

```sql
WITH recent_events AS (
    SELECT e.*, p.player_id
    FROM events AS e
    JOIN event_participations AS p ON p.event_id = e.event_id
    JOIN players AS u ON u.player_id = p.player_id
    WHERE u.steam_id = :steam_id
      AND EXISTS (
          SELECT 1
          FROM session_results AS r
          WHERE r.event_id = e.event_id
            AND r.player_id = p.player_id
            AND r.session_type = 'race'
      )
    ORDER BY e.event_at DESC, e.event_id DESC
    LIMIT :limit
)
SELECT e.event_id, e.event_at, e.title, e.track,
       p.player_id, p.display_name, p.car_class AS entered_class,
       p.dr_adjustment, p.sr_adjustment, p.joker_played,
       r.session_type, r.session_no, r.race_no,
       r.position, r.class_position, r.grid_position,
       r.class_grid_position, r.car_type, r.car_class,
       r.completed_laps, r.best_lap_us, r.finish_time_us, r.finish_status
FROM recent_events AS e
JOIN event_participations AS p
  ON p.event_id = e.event_id AND p.player_id = e.player_id
JOIN session_results AS r
  ON r.event_id = p.event_id AND r.player_id = p.player_id
ORDER BY e.event_at DESC, e.event_id DESC,
         CASE r.session_type
             WHEN 'practice' THEN 1
             WHEN 'qualifying' THEN 2
             WHEN 'warmup' THEN 3
             ELSE 4
         END,
         r.session_no, r.race_no;
```

This returns one row per session race block, so more than N rows may be returned.
Group by `event_id` for an event list. Qualification-only events can be stored but
are excluded from this list. The CLI returns grouped event objects with a
`results` array instead of these flat rows.

### Recent rating history for a graph

Read observations in the half-open interval **`[from_us, to_us)`**, ordered by
observation timestamp and then snapshot ID.

```sql
SELECT s.snapshot_id, s.observed_at,
       s.dr_elo, s.dr_rank, s.dr_tier, s.dr_progress,
       s.sr_rating, s.sr_rank, s.sr_tier, s.sr_progress
FROM rating_snapshots AS s
JOIN players AS u ON u.player_id = s.player_id
WHERE u.steam_id = :steam_id
  AND s.observed_at >= :from_us
  AND s.observed_at < :to_us
ORDER BY s.observed_at, s.snapshot_id;
```

Use observation time, not event time, on the horizontal axis. Gaps mean no
observation; the database does not interpolate them. The CLI accepts RFC3339
boundaries and rounds both bounds **up** to integer microseconds so that the
half-open interval remains correct against stored timestamps, even for
sub-microsecond input boundaries.

### Reference rating at an event's timestamp

Find the same player's latest observation **at or before** `events.event_at`.
Resolve it when reading; do not copy the value or snapshot ID into `events` or
`event_participations`.

```sql
SELECT e.event_id, e.event_at,
       s.snapshot_id AS reference_snapshot_id,
       s.observed_at AS reference_observed_at,
       e.event_at - s.observed_at AS observation_age_us,
       s.dr_elo AS reference_dr_elo,
       s.sr_rating AS reference_sr_rating
FROM events AS e
JOIN event_participations AS p ON p.event_id = e.event_id
JOIN players AS u ON u.player_id = p.player_id
LEFT JOIN rating_snapshots AS s
  ON s.snapshot_id = (
      SELECT prior.snapshot_id
      FROM rating_snapshots AS prior
      WHERE prior.player_id = p.player_id
        AND prior.observed_at <= e.event_at
      ORDER BY prior.observed_at DESC, prior.snapshot_id DESC
      LIMIT 1
  )
WHERE u.steam_id = :steam_id
  AND e.event_id = :event_id;
```

- Equality is allowed. For timestamp ties, the greatest snapshot ID wins.
- If no earlier observation exists, the reference columns are `NULL`; the event
  row is retained by the LEFT JOIN. Future or other players' observations do not
  qualify.
- Show `reference_observed_at` or `observation_age_us` alongside the rating. It is
  the last known value, not a confirmed post-race value.
- Adding a previously missing historical observation can change the reference
  on the next read, without updating any event rows.
- A difference between observations covering several races is an interval
  change, not a known per-race gain. A current observation cannot reconstruct
  ratings for races before observation history began.

The CLI uses a read transaction for its event selection, session reads, and
reference lookups. External consumers issuing separate queries should also use
a read transaction when those results must describe one consistent database view.

## Writes and transactions

The importer first parses the entire input file. Every event must contain
exactly one `drivers` entry matching the selected Steam ID and at least one
matching session result. Duplicate matches within a session race block are
rejected. Names and opponents' records are never substitutes for the tracked
player's information.

One import transaction checks the archive's Steam ID before upserting the player,
events, and participations and replacing session results. A different ID, an
existing multi-user archive, or any later failure aborts the transaction without
changing existing archive data. An empty array or null results page imports zero
events but still checks the account identity.

Online sync saves a complete capture before ingestion. The sanitized profile and
its original observation timestamp are linked by a versioned manifest. One DB
transaction imports events and records the capture marker. A new capture adds a
rating only if its values differ from the preceding stored observation.
The marker is committed even if no snapshot is needed, so replay cannot later
fabricate an observation for that capture. Failures roll back the entire import,
including the marker. Results-only files import no ratings. The filesystem and
SQLite do not share a transaction: a DB failure retains the capture for
`sync --skip-download --input ...`.

## Migrations and backups

- SQL files in `db/migrations/` are embedded in the binary and applied with
  goose by `sync` or explicit `db migrate`.
- `races`, `ratings`, and `serve` use SQLite `mode=ro`, verify the schema version,
  and never create or migrate a database. Missing/outdated schemas require an
  explicit write operation before reading.
- goose records applied versions in its own management table, in addition to
  the six application tables. Do not edit an applied migration; add a new file.
- The initial Down migration drops all six application tables. It cannot restore
  deleted data, and the archive CLI does not expose a Down command.
- Back up before destructive schema changes. Copy the database after stopping
  writers and closing their connections, or use SQLite's backup API for a live
  database. Keep the separate source JSON/profile files with the backup.
- Keep source files immutable. Stored paths are not content hashes. Result JSON
  also contains other participants' names and Steam IDs; review both database
  and source files before sharing them.
