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

// RacesInput carries the raw query strings shared by the CLI and HTTP API.
// Unlike ratings, empty From and To mean unbounded, not a default period.
type RacesInput struct {
	From      string
	To        string
	Track     string
	Class     string
	EventType string
	Limit     string
	Offset    string
}

func (s Service) Races(ctx context.Context, input RacesInput) ([]storage.Race, error) {
	limitText := input.Limit
	if limitText == "" {
		limitText = "10"
	}
	limit, err := strconv.Atoi(limitText)
	if err != nil || limit <= 0 {
		return nil, &InputError{Message: "limit must be a positive integer"}
	}
	offsetText := input.Offset
	if offsetText == "" {
		offsetText = "0"
	}
	offset, err := strconv.Atoi(offsetText)
	if err != nil || offset < 0 {
		return nil, &InputError{Message: "offset must be a non-negative integer"}
	}
	options, err := s.raceFilters(input.From, input.To, input.Track, input.Class, input.EventType)
	if err != nil {
		return nil, err
	}
	steamID, err := s.Store.ResolveSteamID(ctx)
	if err != nil {
		return nil, err
	}
	options.Limit, options.Offset = limit, offset
	return s.Store.LatestRaces(ctx, steamID, options)
}

// TrackStatsInput carries the raw filter strings for the per-track
// aggregation. There is no paging: every matching race is aggregated.
type TrackStatsInput struct {
	From      string
	To        string
	Track     string
	Class     string
	EventType string
}

// TrackStats aggregates the account's races per track. The filters match
// Races exactly; the result never depends on list paging.
func (s Service) TrackStats(ctx context.Context, input TrackStatsInput) ([]storage.TrackStats, error) {
	options, err := s.raceFilters(input.From, input.To, input.Track, input.Class, input.EventType)
	if err != nil {
		return nil, err
	}
	steamID, err := s.Store.ResolveSteamID(ctx)
	if err != nil {
		return nil, err
	}
	return s.Store.TrackStats(ctx, steamID, options)
}

// RaceFilterValues lists the distinct filter values the account's race
// history can match, for building filter controls.
func (s Service) RaceFilterValues(ctx context.Context) (storage.RaceFilterValues, error) {
	steamID, err := s.Store.ResolveSteamID(ctx)
	if err != nil {
		return storage.RaceFilterValues{}, err
	}
	return s.Store.RaceFilterValues(ctx, steamID)
}

// raceFilters validates the filter strings shared by every race selection.
// Empty strings disable their filter; empty From and To mean unbounded.
func (s Service) raceFilters(fromText, toText, track, class, eventType string) (storage.RaceOptions, error) {
	options := storage.RaceOptions{Track: track, Class: class, EventType: eventType}
	if fromText != "" {
		value, err := parseTime(fromText, time.Time{})
		if err != nil {
			return options, &InputError{Message: "from must be YYYY-MM-DD or RFC3339"}
		}
		options.From = &value
	}
	if toText != "" {
		value, err := parseTime(toText, time.Time{})
		if err != nil {
			return options, &InputError{Message: "to must be YYYY-MM-DD or RFC3339"}
		}
		options.To = &value
	}
	if options.From != nil && options.To != nil && !options.From.Before(*options.To) {
		return options, &InputError{Message: "from must precede to (from inclusive, to exclusive)"}
	}
	return options, nil
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
