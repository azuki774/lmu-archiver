package ingest

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/azuki774/lmu-archiver/internal/storage"
)

// playerProfile is an explicit allowlist, not the raw authentication response.
// Unknown fields (including credentials nested in player/rank/platform) are lost.
type playerProfile struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Username  string `json:"username,omitempty"`
	Platforms []struct {
		ID       string `json:"id"`
		Platform string `json:"platform"`
	} `json:"platforms"`
	DR rank `json:"driverRank"`
	SR rank `json:"safetyRank"`
}

// Profile returns the authenticated account, exact observation, and sanitized
// profile JSON. Missing exact values are errors; a present numeric zero is valid.
func Profile(raw json.RawMessage, observedAt time.Time, rawPath string) (storage.Player, storage.RatingSnapshot, []byte, error) {
	var p playerProfile
	var player storage.Player
	var snapshot storage.RatingSnapshot
	if err := json.Unmarshal(raw, &p); err != nil {
		return player, snapshot, nil, errors.New("authentication profile is missing or invalid")
	}
	steamID := ""
	for _, platform := range p.Platforms {
		if platform.Platform != "steam" {
			continue
		}
		if steamID != "" || strings.TrimSpace(platform.ID) == "" {
			return player, snapshot, nil, errors.New("profile has ambiguous or missing Steam identity")
		}
		steamID = platform.ID
	}
	if steamID == "" {
		return player, snapshot, nil, errors.New("profile has no Steam identity")
	}
	if p.DR.Elo == nil || p.SR.Rating == nil || p.DR.Rank == nil || p.SR.Rank == nil || p.DR.Tier == nil || p.SR.Tier == nil || p.DR.Progress == nil || p.SR.Progress == nil {
		return player, snapshot, nil, errors.New("profile is missing exact DR/SR fields")
	}
	for _, v := range []float64{*p.DR.Elo, *p.DR.Progress, *p.SR.Rating, *p.SR.Progress} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return player, snapshot, nil, errors.New("profile ratings must be finite")
		}
	}
	name := p.Name
	if name == "" {
		name = p.Username
	}
	player = storage.Player{SteamID: steamID, RaceOSPlayerID: p.ID, Name: name}
	snapshot = storage.RatingSnapshot{ObservedAt: observedAt.UTC(), Source: "authenticate", RawPath: rawPath, DRElo: *p.DR.Elo, DRRank: *p.DR.Rank, DRTier: *p.DR.Tier, DRProgress: *p.DR.Progress, SRRating: *p.SR.Rating, SRRank: *p.SR.Rank, SRTier: *p.SR.Tier, SRProgress: *p.SR.Progress}
	safe, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return player, snapshot, nil, errors.New("cannot encode sanitized profile")
	}
	return player, snapshot, append(safe, '\n'), nil
}
