// Package query defines the read operations shared by the CLI and HTTP API.
package query

import (
	"context"
	"strconv"
	"time"

	"github.com/azuki774/lmu-archiver/internal/storage"
)

type InputError struct{ Message string }

func (e *InputError) Error() string { return e.Message }

// Service reads the single account stored in the archive. The DB owner is
// resolved for every request; callers cannot select another account.
type Service struct {
	Store *storage.Store
}

func (s Service) Races(ctx context.Context, limitText string) ([]storage.Race, error) {
	if limitText == "" {
		limitText = "10"
	}
	limit, err := strconv.Atoi(limitText)
	if err != nil || limit <= 0 {
		return nil, &InputError{Message: "limit must be a positive integer"}
	}
	steamID, err := s.Store.ResolveSteamID(ctx)
	if err != nil {
		return nil, err
	}
	return s.Store.LatestRaces(ctx, steamID, limit)
}

func (s Service) Ratings(ctx context.Context, fromText, toText string) ([]storage.RatingSnapshot, error) {
	now := time.Now().UTC()
	from, err := parseTime(fromText, now.AddDate(0, 0, -30))
	if err != nil {
		return nil, &InputError{Message: "from must be YYYY-MM-DD or RFC3339"}
	}
	to, err := parseTime(toText, now)
	if err != nil {
		return nil, &InputError{Message: "to must be YYYY-MM-DD or RFC3339"}
	}
	if !from.Before(to) {
		return nil, &InputError{Message: "from must precede to (from inclusive, to exclusive)"}
	}
	steamID, err := s.Store.ResolveSteamID(ctx)
	if err != nil {
		return nil, err
	}
	return s.Store.Ratings(ctx, steamID, from, to)
}

func (s Service) LatestRatings(ctx context.Context, limitText string) ([]storage.RatingSnapshot, error) {
	if limitText == "" {
		limitText = "10"
	}
	limit, err := strconv.Atoi(limitText)
	if err != nil || limit <= 0 {
		return nil, &InputError{Message: "limit must be a positive integer"}
	}
	steamID, err := s.Store.ResolveSteamID(ctx)
	if err != nil {
		return nil, err
	}
	return s.Store.LatestRatings(ctx, steamID, limit)
}

func parseTime(text string, fallback time.Time) (time.Time, error) {
	if text == "" {
		return fallback, nil
	}
	if value, err := time.Parse(time.RFC3339Nano, text); err == nil {
		return value, nil
	}
	return time.Parse(time.DateOnly, text)
}
