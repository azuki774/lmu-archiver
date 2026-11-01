-- +goose Up
CREATE TABLE capture_observations (
    capture_id TEXT NOT NULL CHECK (length(trim(capture_id)) > 0),
    player_id INTEGER NOT NULL REFERENCES players(player_id),
    PRIMARY KEY (capture_id, player_id)
) STRICT;

-- +goose Down
DROP TABLE capture_observations;
