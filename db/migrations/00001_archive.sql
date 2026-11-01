-- +goose Up
CREATE TABLE players (
    player_id INTEGER PRIMARY KEY,
    steam_id TEXT NOT NULL UNIQUE CHECK (length(trim(steam_id)) > 0),
    raceos_player_id TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL
) STRICT;

CREATE TABLE events (
    event_id TEXT PRIMARY KEY NOT NULL CHECK (length(trim(event_id)) > 0),
    series_id TEXT NOT NULL,
    event_at INTEGER NOT NULL,
    title TEXT NOT NULL,
    event_type TEXT NOT NULL,
    tier TEXT NOT NULL,
    track TEXT NOT NULL,
    track_scene TEXT NOT NULL,
    split_no TEXT NOT NULL,
    official INTEGER NOT NULL CHECK (official IN (0, 1)),
    team_event INTEGER NOT NULL CHECK (team_event IN (0, 1)),
    raw_path TEXT NOT NULL,
    imported_at INTEGER NOT NULL
) STRICT;

CREATE TABLE event_participations (
    event_id TEXT NOT NULL REFERENCES events(event_id),
    player_id INTEGER NOT NULL REFERENCES players(player_id),
    display_name TEXT NOT NULL,
    car_class TEXT NOT NULL,
    dr_rank TEXT,
    dr_tier INTEGER,
    dr_progress REAL,
    sr_rank TEXT,
    sr_tier INTEGER,
    sr_progress REAL,
    dr_adjustment INTEGER CHECK (dr_adjustment BETWEEN -3 AND 3),
    sr_adjustment INTEGER CHECK (sr_adjustment BETWEEN -3 AND 3),
    joker_played INTEGER NOT NULL CHECK (joker_played IN (0, 1)),
    PRIMARY KEY (event_id, player_id)
) STRICT;

CREATE TABLE session_results (
    event_id TEXT NOT NULL,
    player_id INTEGER NOT NULL,
    session_type TEXT NOT NULL CHECK (session_type IN ('practice', 'qualifying', 'warmup', 'race')),
    session_no TEXT NOT NULL,
    race_no INTEGER NOT NULL CHECK (race_no > 0),
    position INTEGER CHECK (position > 0),
    class_position INTEGER CHECK (class_position > 0),
    grid_position INTEGER CHECK (grid_position > 0),
    class_grid_position INTEGER CHECK (class_grid_position > 0),
    car_type TEXT NOT NULL,
    car_class TEXT NOT NULL,
    car_number TEXT NOT NULL,
    team_name TEXT NOT NULL,
    completed_laps INTEGER CHECK (completed_laps >= 0),
    best_lap_us INTEGER CHECK (best_lap_us >= 0),
    finish_time_us INTEGER CHECK (finish_time_us >= 0),
    finish_status TEXT NOT NULL,
    PRIMARY KEY (event_id, player_id, session_type, session_no, race_no),
    FOREIGN KEY (event_id, player_id) REFERENCES event_participations(event_id, player_id)
) STRICT;

CREATE TABLE rating_snapshots (
    snapshot_id INTEGER PRIMARY KEY,
    player_id INTEGER NOT NULL REFERENCES players(player_id),
    observed_at INTEGER NOT NULL,
    source TEXT NOT NULL CHECK (length(trim(source)) > 0),
    raw_path TEXT NOT NULL,
    dr_elo REAL NOT NULL,
    dr_rank TEXT NOT NULL,
    dr_tier INTEGER NOT NULL,
    dr_progress REAL NOT NULL,
    sr_rating REAL NOT NULL,
    sr_rank TEXT NOT NULL,
    sr_tier INTEGER NOT NULL,
    sr_progress REAL NOT NULL
) STRICT;

CREATE TABLE capture_observations (
    capture_id TEXT NOT NULL CHECK (length(trim(capture_id)) > 0),
    player_id INTEGER NOT NULL REFERENCES players(player_id),
    PRIMARY KEY (capture_id, player_id)
) STRICT;

CREATE INDEX idx_events_time ON events(event_at, event_id);
CREATE INDEX idx_participations_player ON event_participations(player_id, event_id);
CREATE INDEX idx_rating_snapshots_player_time ON rating_snapshots(player_id, observed_at);

-- +goose Down
DROP TABLE capture_observations;
DROP TABLE rating_snapshots;
DROP TABLE session_results;
DROP TABLE event_participations;
DROP TABLE events;
DROP TABLE players;
