package ingest

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

const ownEvent = `{"eventId":"event","date":"2026-01-01T12:00:00Z","drivers":[{"steamId":"self","name":"Same Name","class":"GT3","driverRank":{"elo":0,"rank":"Silver","tier":1,"progress":20},"driverRankAdjustment":1},{"steamId":"other","name":"Same Name"}],"race":{"sessionNo":"1","races":[{"results":[{"steamId":"other","name":"Same Name","position":"1"},{"steamId":"self","position":"2","classPosition":"1","gridPos":"","bestLapTime":"99.8350","finishTime":"1830.9375","Laps":18,"carNumber":"032","finishStatus":"Finished Normally"}]}]}}`

func TestTrackedResultIdentityAndMissingValues(t *testing.T) {
	at := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	player, events, err := Results(strings.NewReader("["+ownEvent+"]"), "self", "/raw/results.json", at)
	if err != nil {
		t.Fatal(err)
	}
	if player.SteamID != "self" || player.Name != "Same Name" || len(events) != 1 {
		t.Fatalf("wrong tracked account: %+v", player)
	}
	event := events[0]
	row := event.Results[0]
	if *row.Position != 2 || *row.ClassPosition != 1 || row.GridPosition != nil {
		t.Fatalf("wrong result or missing field: %+v", row)
	}
	if *row.BestLapUS != 99_835_000 || *row.FinishTimeUS != 1_830_937_500 || row.CarNumber != "032" {
		t.Fatalf("numeric precision or identifier lost: %+v", row)
	}
	if row.RaceNo != 1 || row.SessionNo != "1" || *event.Participation.DRAdjustment != 1 || event.Participation.SRProgress != nil {
		t.Fatal("event/session metadata was lost")
	}
}

func TestRejectAmbiguousOrIncompleteOwnResults(t *testing.T) {
	cases := map[string]string{
		"missing own driver":   "[" + strings.Replace(ownEvent, `"steamId":"self"`, `"steamId":"unknown"`, 1) + "]",
		"missing own result":   "[" + strings.Replace(ownEvent, `"steamId":"self","position"`, `"steamId":"unknown","position"`, 1) + "]",
		"duplicate own driver": "[" + strings.Replace(ownEvent, `"drivers":[`, `"drivers":[{"steamId":"self"},`, 1) + "]",
		"duplicate own result": "[" + strings.Replace(ownEvent, `"results":[`, `"results":[{"steamId":"self"},`, 1) + "]",
		"duplicate event":      "[" + ownEvent + "," + ownEvent + "]",
		"later invalid event":  "[" + ownEvent + `,{"eventId":"second","date":"2026-01-02T00:00:00Z","drivers":[]}]`,
		"trailing document":    "[" + ownEvent + "] {}",
		"truncated document":   "[" + ownEvent,
		"invalid position":     "[" + strings.Replace(ownEvent, `"position":"2"`, `"position":"bad"`, 1) + "]",
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			_, events, err := Results(strings.NewReader(input), "self", "raw", time.Now())
			if err == nil || events != nil {
				t.Fatalf("invalid input returned partial events: %v", err)
			}
		})
	}
}

func TestDecimalSecondsBoundaries(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want int64
	}{{"0", 0}, {"0.000001", 1}, {"99.8350", 99_835_000}, {"1.123456000", 1_123_456}, {"9223372036854.775807", 9223372036854775807}} {
		got, err := secondsUS(tc.raw)
		if err != nil || got == nil || *got != tc.want {
			t.Fatalf("%q: %v %v", tc.raw, got, err)
		}
	}
	for _, raw := range []string{"-1", "NaN", "1e2", "1/2", "1.", ".1", "1.0000001", "9223372036854.775808", "999999999999999999999"} {
		if _, err := secondsUS(raw); err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
	got, err := secondsUS("")
	if err != nil || got != nil {
		t.Fatal("empty time should be missing")
	}
}

const profileJSON = `{"id":"player-id","name":"Driver","accessToken":"profile-secret","platforms":[{"id":"self","platform":"steam","token":"platform-secret"}],"driverRank":{"elo":1500.25,"rank":"Silver","tier":1,"progress":25.5,"token":"rank-secret"},"safetyRank":{"rating":4.25,"rank":"Gold","tier":2,"progress":80.125}}`

func TestSanitizedExactProfile(t *testing.T) {
	at := time.Now().UTC()
	player, snapshot, safe, err := Profile(json.RawMessage(profileJSON), at, "raw.json")
	if err != nil {
		t.Fatal(err)
	}
	if player.SteamID != "self" || player.RaceOSPlayerID != "player-id" || snapshot.DRElo != 1500.25 || snapshot.SRRating != 4.25 || snapshot.SRProgress != 80.125 || !snapshot.ObservedAt.Equal(at) {
		t.Fatal("exact profile values lost")
	}
	if strings.Contains(string(safe), "secret") || strings.Contains(string(safe), "accessToken") {
		t.Fatal("credential fields survived sanitization")
	}
	zero := strings.Replace(profileJSON, `"elo":1500.25`, `"elo":0`, 1)
	_, value, _, err := Profile(json.RawMessage(zero), at, "")
	if err != nil || value.DRElo != 0 {
		t.Fatal("present zero confused with missing rating")
	}
	for _, raw := range []string{`null`, `{}`, strings.Replace(profileJSON, `"elo":1500.25,`, "", 1), strings.Replace(profileJSON, `"platform":"steam"`, `"platform":"unknown"`, 1)} {
		if _, _, safe, err := Profile(json.RawMessage(raw), at, ""); err == nil || safe != nil {
			t.Fatal("missing exact rating/identity accepted")
		}
	}
}
