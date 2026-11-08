// Package storage persists race results and rating observations in SQLite.
package storage

import "time"

// Player identifies a tracked account independently of its display name.
type Player struct {
	ID             int64  `json:"id"`
	SteamID        string `json:"steamId"`
	RaceOSPlayerID string `json:"raceosPlayerId,omitempty"`
	Name           string `json:"name"`
}

// Event contains information shared by every participant in an event.
type Event struct {
	ID         string    `json:"eventId"`
	SeriesID   string    `json:"seriesId"`
	At         time.Time `json:"eventAt"`
	Title      string    `json:"title"`
	Type       string    `json:"eventType"`
	Tier       string    `json:"tier"`
	Track      string    `json:"track"`
	TrackScene string    `json:"trackScene"`
	SplitNo    string    `json:"splitNo"`
	Official   bool      `json:"official"`
	TeamEvent  bool      `json:"teamEvent"`
	RawPath    string    `json:"rawPath"`
	ImportedAt time.Time `json:"importedAt"`
}

// Participation contains event-level data, not exact rating observations.
type Participation struct {
	EventID      string   `json:"eventId"`
	PlayerID     int64    `json:"playerId"`
	Name         string   `json:"name"`
	CarClass     string   `json:"carClass"`
	DRRank       *string  `json:"drRank"`
	DRTier       *int     `json:"drTier"`
	DRProgress   *float64 `json:"drProgress"`
	SRRank       *string  `json:"srRank"`
	SRTier       *int     `json:"srTier"`
	SRProgress   *float64 `json:"srProgress"`
	DRAdjustment *int     `json:"drAdjustment"`
	SRAdjustment *int     `json:"srAdjustment"`
	JokerPlayed  bool     `json:"jokerPlayed"`
}

// SessionResult contains the tracked player's result in one session race block.
// Missing API numbers are represented by nil, never coerced to zero.
type SessionResult struct {
	EventID           string `json:"eventId"`
	PlayerID          int64  `json:"playerId"`
	SessionType       string `json:"sessionType"`
	SessionNo         string `json:"sessionNo"`
	RaceNo            int    `json:"raceNo"`
	Position          *int   `json:"position"`
	ClassPosition     *int   `json:"classPosition"`
	GridPosition      *int   `json:"gridPosition"`
	ClassGridPosition *int   `json:"classGridPosition"`
	CarType           string `json:"carType"`
	CarClass          string `json:"carClass"`
	CarNumber         string `json:"carNumber"`
	TeamName          string `json:"teamName"`
	CompletedLaps     *int   `json:"completedLaps"`
	BestLapUS         *int64 `json:"bestLapUs"`
	FinishTimeUS      *int64 `json:"finishTimeUs"`
	FinishStatus      string `json:"finishStatus"`
}

// EventData is imported atomically with its tracked participant and sessions.
type EventData struct {
	Event         Event
	Participation Participation
	Results       []SessionResult
}

// RatingSnapshot is a point-in-time observation, not a per-race delta.
type RatingSnapshot struct {
	ID         int64     `json:"snapshotId"`
	PlayerID   int64     `json:"playerId"`
	ObservedAt time.Time `json:"observedAt"`
	Source     string    `json:"source"`
	RawPath    string    `json:"rawPath"`
	DRElo      float64   `json:"drElo"`
	DRRank     string    `json:"drRank"`
	DRTier     int       `json:"drTier"`
	DRProgress float64   `json:"drProgress"`
	SRRating   float64   `json:"srRating"`
	SRRank     string    `json:"srRank"`
	SRTier     int       `json:"srTier"`
	SRProgress float64   `json:"srProgress"`
}

// Race combines an event, the tracked player's sessions, and an optional
// latest observation at or before Event.At. ReferenceRating is never copied
// into event or participation rows and is not the confirmed post-race rating.
type Race struct {
	Event           Event           `json:"event"`
	Participation   Participation   `json:"participation"`
	Results         []SessionResult `json:"results"`
	ReferenceRating *RatingSnapshot `json:"referenceRating"`
}

// RaceOptions filters and bounds the events selected for one tracked player.
// From is inclusive and To exclusive; nil and empty values do not constrain
// the selection. Track matches the event, Class the tracked player's
// participation, and EventType the event, all by exact equality.
type RaceOptions struct {
	Limit     int
	Offset    int
	From      *time.Time
	To        *time.Time
	Track     string
	Class     string
	EventType string
}

// PositionStats aggregates one ranking kind. Overall and class rankings stay
// separate so a view cannot blend them. Null means no positioned result
// exists, not a zero value.
type PositionStats struct {
	Wins         int      `json:"wins"`
	Podiums      int      `json:"podiums"`
	AvgPosition  *float64 `json:"avgPosition"`
	BestPosition *int     `json:"bestPosition"`
}

// TrackStats aggregates the tracked player's race sessions per track across
// every event matching the filters. Races counts distinct events and Starts
// race session blocks; Limit and Offset in RaceOptions never apply here.
// Overall uses position, Class uses class_position. BestLapUs is the fastest
// race lap or null when no lap was recorded.
type TrackStats struct {
	Track     string        `json:"track"`
	Races     int           `json:"races"`
	Starts    int           `json:"starts"`
	Overall   PositionStats `json:"overall"`
	Class     PositionStats `json:"class"`
	BestLapUs *int64        `json:"bestLapUs"`
}

// RaceFilterValues lists the distinct filter values the account's race
// history can match. Only events with the tracked player's own race sessions
// are considered, so every offered value can produce matches.
type RaceFilterValues struct {
	Tracks     []string `json:"tracks"`
	Classes    []string `json:"classes"`
	EventTypes []string `json:"eventTypes"`
}
