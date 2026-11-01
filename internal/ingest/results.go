// Package ingest converts RaceOS JSON into typed archive records.
package ingest

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/azuki774/lmu-archiver/internal/storage"
)

type rank struct {
	Rank     *string  `json:"rank"`
	Tier     *int     `json:"tier"`
	Progress *float64 `json:"progress"`
	Elo      *float64 `json:"elo,omitempty"`
	Rating   *float64 `json:"rating,omitempty"`
}
type driver struct {
	SteamID      string `json:"steamId"`
	Name         string `json:"name"`
	Class        string `json:"class"`
	DR           rank   `json:"driverRank"`
	SR           rank   `json:"safetyRank"`
	DRAdjustment *int   `json:"driverRankAdjustment"`
	SRAdjustment *int   `json:"safetyRankAdjustment"`
	Joker        bool   `json:"jokerPlayed"`
}
type result struct {
	SteamID           string `json:"steamId"`
	Position          string `json:"position"`
	ClassPosition     string `json:"classPosition"`
	GridPosition      string `json:"gridPos"`
	ClassGridPosition string `json:"classGridPos"`
	CarType           string `json:"carType"`
	CarClass          string `json:"carClass"`
	CarNumber         string `json:"carNumber"`
	TeamName          string `json:"teamName"`
	Laps              *int   `json:"Laps"`
	BestLap           string `json:"bestLapTime"`
	FinishTime        string `json:"finishTime"`
	FinishStatus      string `json:"finishStatus"`
}
type session struct {
	Number string `json:"sessionNo"`
	Races  []struct {
		Results []result `json:"results"`
	} `json:"races"`
}
type sourceEvent struct {
	ID         string    `json:"eventId"`
	SeriesID   string    `json:"seriesId"`
	Date       time.Time `json:"date"`
	Title      string    `json:"title"`
	Type       string    `json:"eventType"`
	Tier       string    `json:"tier"`
	Track      string    `json:"track"`
	TrackScene string    `json:"trackScene"`
	SplitNo    string    `json:"splitNo"`
	Official   bool      `json:"official"`
	TeamEvent  bool      `json:"teamEvent"`
	Drivers    []driver  `json:"drivers"`
	Practice   *session  `json:"practice"`
	Qualifying *session  `json:"qualifying"`
	Warmup     *session  `json:"warmup"`
	Race       *session  `json:"race"`
}

// Results decodes a complete array (or an empty/null page) before persistence.
// Every event must identify the tracked driver and at least one own session.
func Results(input io.Reader, steamID, rawPath string, importedAt time.Time) (storage.Player, []storage.EventData, error) {
	player := storage.Player{SteamID: steamID}
	if strings.TrimSpace(steamID) == "" {
		return player, nil, errors.New("Steam ID is required")
	}
	var source []sourceEvent
	decoder := json.NewDecoder(input)
	if err := decoder.Decode(&source); err != nil {
		return player, nil, errors.New("results input must be a JSON array or null")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return player, nil, errors.New("results input contains trailing JSON or invalid data")
	}
	events := make([]storage.EventData, 0, len(source))
	seen := make(map[string]bool, len(source))
	for i := range source {
		e := &source[i]
		if e.ID == "" || e.Date.IsZero() || seen[e.ID] {
			return player, nil, fmt.Errorf("event %d has a missing/duplicate ID or missing timestamp", i+1)
		}
		seen[e.ID] = true
		var own *driver
		for j := range e.Drivers {
			if e.Drivers[j].SteamID == steamID {
				if own != nil {
					return player, nil, fmt.Errorf("event %d has duplicate tracked driver entries", i+1)
				}
				own = &e.Drivers[j]
			}
		}
		if own == nil {
			return player, nil, fmt.Errorf("event %d does not identify the tracked driver by Steam ID", i+1)
		}
		if player.Name == "" {
			player.Name = own.Name
		}
		data := storage.EventData{
			Event:         storage.Event{ID: e.ID, SeriesID: e.SeriesID, At: e.Date.UTC(), Title: e.Title, Type: e.Type, Tier: e.Tier, Track: e.Track, TrackScene: e.TrackScene, SplitNo: e.SplitNo, Official: e.Official, TeamEvent: e.TeamEvent, RawPath: rawPath, ImportedAt: importedAt.UTC()},
			Participation: storage.Participation{EventID: e.ID, Name: own.Name, CarClass: own.Class, DRRank: own.DR.Rank, DRTier: own.DR.Tier, DRProgress: own.DR.Progress, SRRank: own.SR.Rank, SRTier: own.SR.Tier, SRProgress: own.SR.Progress, DRAdjustment: own.DRAdjustment, SRAdjustment: own.SRAdjustment, JokerPlayed: own.Joker},
		}
		for _, part := range []struct {
			kind  string
			value *session
		}{{"practice", e.Practice}, {"qualifying", e.Qualifying}, {"warmup", e.Warmup}, {"race", e.Race}} {
			if part.value == nil {
				continue
			}
			for block := range part.value.Races {
				found := false
				for j := range part.value.Races[block].Results {
					row := &part.value.Races[block].Results[j]
					if row.SteamID != steamID {
						continue
					}
					if found {
						return player, nil, fmt.Errorf("event %d %s block %d has duplicate tracked results", i+1, part.kind, block+1)
					}
					found = true
					r, err := convertResult(row)
					if err != nil {
						return player, nil, fmt.Errorf("event %d %s block %d: %w", i+1, part.kind, block+1, err)
					}
					r.EventID = e.ID
					r.SessionType = part.kind
					r.SessionNo = part.value.Number
					r.RaceNo = block + 1
					data.Results = append(data.Results, r)
				}
			}
		}
		if len(data.Results) == 0 {
			return player, nil, fmt.Errorf("event %d has no tracked session results", i+1)
		}
		events = append(events, data)
	}
	return player, events, nil
}

func convertResult(row *result) (storage.SessionResult, error) {
	r := storage.SessionResult{CarType: row.CarType, CarClass: row.CarClass, CarNumber: row.CarNumber, TeamName: row.TeamName, CompletedLaps: row.Laps, FinishStatus: row.FinishStatus}
	for _, field := range []struct {
		name, value string
		target      **int
	}{{"position", row.Position, &r.Position}, {"classPosition", row.ClassPosition, &r.ClassPosition}, {"gridPos", row.GridPosition, &r.GridPosition}, {"classGridPos", row.ClassGridPosition, &r.ClassGridPosition}} {
		if field.value == "" {
			continue
		}
		value, err := strconv.Atoi(field.value)
		if err != nil || value <= 0 {
			return r, fmt.Errorf("%s must be a positive integer or empty", field.name)
		}
		*field.target = &value
	}
	if row.Laps != nil && *row.Laps < 0 {
		return r, errors.New("Laps must be nonnegative")
	}
	var err error
	r.BestLapUS, err = secondsUS(row.BestLap)
	if err != nil {
		return r, fmt.Errorf("bestLapTime: %w", err)
	}
	r.FinishTimeUS, err = secondsUS(row.FinishTime)
	if err != nil {
		return r, fmt.Errorf("finishTime: %w", err)
	}
	return r, nil
}

// secondsUS converts decimal seconds exactly, rejecting nonzero precision below
// a microsecond rather than silently rounding archived measurements.
func secondsUS(raw string) (*int64, error) {
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ".")
	if len(parts) > 2 || parts[0] == "" {
		return nil, errors.New("expected nonnegative decimal seconds")
	}
	for _, part := range parts {
		if part == "" {
			return nil, errors.New("expected nonnegative decimal seconds")
		}
		for _, c := range part {
			if c < '0' || c > '9' {
				return nil, errors.New("expected nonnegative decimal seconds")
			}
		}
	}
	seconds, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || seconds > math.MaxInt64/1_000_000 {
		return nil, errors.New("time exceeds microsecond range")
	}
	var fraction int64
	if len(parts) == 2 {
		digits := strings.TrimRight(parts[1], "0")
		if len(digits) > 6 {
			return nil, errors.New("time has sub-microsecond precision")
		}
		if digits != "" {
			fraction, _ = strconv.ParseInt(digits+strings.Repeat("0", 6-len(digits)), 10, 64)
		}
	}
	if seconds == math.MaxInt64/1_000_000 && fraction > math.MaxInt64%1_000_000 {
		return nil, errors.New("time exceeds microsecond range")
	}
	value := seconds*1_000_000 + fraction
	return &value, nil
}
