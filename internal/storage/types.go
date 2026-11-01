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
