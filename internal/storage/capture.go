package storage

import (
	"context"
	"errors"
)

// ImportCapture replaces event data and records changed ratings together.
// A stable capture ID marks even unchanged observations as processed.
func (s *Store) ImportCapture(ctx context.Context, player Player, events []EventData, captureID string, rating *RatingSnapshot) (bool, error) {
	if rating != nil && captureID == "" {
		return false, errors.New("rating capture requires an ID")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	playerID, err := upsertPlayer(ctx, tx, player)
	if err != nil {
		return false, err
	}
	if err := importEvents(ctx, tx, playerID, events); err != nil {
		return false, err
	}
	recorded := false
	if rating != nil {
		result, err := tx.ExecContext(ctx, `INSERT INTO capture_observations(capture_id,player_id) VALUES(?,?) ON CONFLICT(capture_id,player_id) DO NOTHING`, captureID, playerID)
		if err != nil {
			return false, err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return false, err
		}
		if count != 0 {
			recorded, err = recordRating(ctx, tx, playerID, *rating)
			if err != nil {
				return false, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return recorded, nil
}
