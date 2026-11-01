package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/azuki774/lmu-archiver/db/migrations"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

// Store owns a SQLite connection pool. Each connection enables foreign keys.
type Store struct{ db *sql.DB }

// Open creates a private archive file if needed and applies embedded migrations.
func Open(ctx context.Context, path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("database path is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(absolute, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: absolute}
	q := url.Values{}
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	provider, err := goose.NewProvider(goose.DialectSQLite3, db, migrations.Files)
	if err == nil {
		_, err = provider.Up(ctx)
	}
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate archive: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func upsertPlayer(ctx context.Context, tx *sql.Tx, p Player) (int64, error) {
	if strings.TrimSpace(p.SteamID) == "" {
		return 0, errors.New("Steam ID is required")
	}
	var id int64
	err := tx.QueryRowContext(ctx, `INSERT INTO players(steam_id,raceos_player_id,display_name) VALUES(?,?,?)
 ON CONFLICT(steam_id) DO UPDATE SET
 raceos_player_id=CASE WHEN excluded.raceos_player_id='' THEN players.raceos_player_id ELSE excluded.raceos_player_id END,
 display_name=CASE WHEN excluded.display_name='' THEN players.display_name ELSE excluded.display_name END
 RETURNING player_id`, p.SteamID, p.RaceOSPlayerID, p.Name).Scan(&id)
	return id, err
}

// ImportEvents imports a complete input atomically. Reimports replace only the
// tracked player's session rows; they cannot delete another player's results.
func (s *Store) ImportEvents(ctx context.Context, player Player, events []EventData) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	playerID, err := upsertPlayer(ctx, tx, player)
	if err != nil {
		return err
	}
	if err := importEvents(ctx, tx, playerID, events); err != nil {
		return err
	}
	return tx.Commit()
}

func importEvents(ctx context.Context, tx *sql.Tx, playerID int64, events []EventData) error {
	eventStmt, err := tx.PrepareContext(ctx, `INSERT INTO events(event_id,series_id,event_at,title,event_type,tier,track,track_scene,split_no,official,team_event,raw_path,imported_at)
 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(event_id) DO UPDATE SET
 series_id=excluded.series_id,event_at=excluded.event_at,title=excluded.title,event_type=excluded.event_type,tier=excluded.tier,
 track=excluded.track,track_scene=excluded.track_scene,split_no=excluded.split_no,official=excluded.official,team_event=excluded.team_event,
 raw_path=excluded.raw_path,imported_at=excluded.imported_at`)
	if err != nil {
		return err
	}
	defer eventStmt.Close()
	participationStmt, err := tx.PrepareContext(ctx, `INSERT INTO event_participations(event_id,player_id,display_name,car_class,dr_rank,dr_tier,dr_progress,sr_rank,sr_tier,sr_progress,dr_adjustment,sr_adjustment,joker_played)
 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(event_id,player_id) DO UPDATE SET
 display_name=excluded.display_name,car_class=excluded.car_class,dr_rank=excluded.dr_rank,dr_tier=excluded.dr_tier,dr_progress=excluded.dr_progress,
 sr_rank=excluded.sr_rank,sr_tier=excluded.sr_tier,sr_progress=excluded.sr_progress,dr_adjustment=excluded.dr_adjustment,
 sr_adjustment=excluded.sr_adjustment,joker_played=excluded.joker_played`)
	if err != nil {
		return err
	}
	defer participationStmt.Close()
	resultStmt, err := tx.PrepareContext(ctx, `INSERT INTO session_results(event_id,player_id,session_type,session_no,race_no,position,class_position,grid_position,class_grid_position,car_type,car_class,car_number,team_name,completed_laps,best_lap_us,finish_time_us,finish_status)
 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer resultStmt.Close()
	for _, item := range events {
		e, p := item.Event, item.Participation
		if e.ID == "" || e.At.IsZero() || e.ImportedAt.IsZero() || p.EventID != e.ID || len(item.Results) == 0 {
			return errors.New("event must have a timestamp, matching participation, and tracked session results")
		}
		if _, err = eventStmt.ExecContext(ctx, e.ID, e.SeriesID, e.At.UnixMicro(), e.Title, e.Type, e.Tier, e.Track, e.TrackScene, e.SplitNo, e.Official, e.TeamEvent, e.RawPath, e.ImportedAt.UnixMicro()); err != nil {
			return fmt.Errorf("save event: %w", err)
		}
		if _, err = participationStmt.ExecContext(ctx, e.ID, playerID, p.Name, p.CarClass, p.DRRank, p.DRTier, p.DRProgress, p.SRRank, p.SRTier, p.SRProgress, p.DRAdjustment, p.SRAdjustment, p.JokerPlayed); err != nil {
			return fmt.Errorf("save participation: %w", err)
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM session_results WHERE event_id=? AND player_id=?`, e.ID, playerID); err != nil {
			return err
		}
		for _, r := range item.Results {
			if r.EventID != e.ID {
				return errors.New("session result event does not match participation")
			}
			if _, err = resultStmt.ExecContext(ctx, e.ID, playerID, r.SessionType, r.SessionNo, r.RaceNo, r.Position, r.ClassPosition, r.GridPosition, r.ClassGridPosition, r.CarType, r.CarClass, r.CarNumber, r.TeamName, r.CompletedLaps, r.BestLapUS, r.FinishTimeUS, r.FinishStatus); err != nil {
				return fmt.Errorf("save session result: %w", err)
			}
		}
	}
	return nil
}

// RecordRating records a change from the preceding stored DR/SR values.
// A snapshot is never backdated to a race automatically.
func (s *Store) RecordRating(ctx context.Context, player Player, r RatingSnapshot) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	playerID, err := upsertPlayer(ctx, tx, player)
	if err != nil {
		return err
	}
	if _, err := recordRating(ctx, tx, playerID, r); err != nil {
		return err
	}
	return tx.Commit()
}

func recordRating(ctx context.Context, tx *sql.Tx, playerID int64, r RatingSnapshot) (bool, error) {
	if r.ObservedAt.IsZero() || strings.TrimSpace(r.Source) == "" {
		return false, errors.New("rating observation requires timestamp and source")
	}
	for _, v := range []float64{r.DRElo, r.DRProgress, r.SRRating, r.SRProgress} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false, errors.New("rating values must be finite")
		}
	}
	// Compare at the observation time, not import time: replaying an older
	// capture must not compare its values against a future rating.
	var previous RatingSnapshot
	err := tx.QueryRowContext(ctx, `SELECT dr_elo,dr_rank,dr_tier,dr_progress,sr_rating,sr_rank,sr_tier,sr_progress
 FROM rating_snapshots WHERE player_id=? AND observed_at<=?
 ORDER BY observed_at DESC,snapshot_id DESC LIMIT 1`, playerID, r.ObservedAt.UnixMicro()).Scan(
		&previous.DRElo, &previous.DRRank, &previous.DRTier, &previous.DRProgress,
		&previous.SRRating, &previous.SRRank, &previous.SRTier, &previous.SRProgress)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if err == nil && previous.DRElo == r.DRElo && previous.DRRank == r.DRRank &&
		previous.DRTier == r.DRTier && previous.DRProgress == r.DRProgress &&
		previous.SRRating == r.SRRating && previous.SRRank == r.SRRank &&
		previous.SRTier == r.SRTier && previous.SRProgress == r.SRProgress {
		return false, nil
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO rating_snapshots(player_id,observed_at,source,raw_path,dr_elo,dr_rank,dr_tier,dr_progress,sr_rating,sr_rank,sr_tier,sr_progress)
 VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, playerID, r.ObservedAt.UnixMicro(), r.Source, r.RawPath, r.DRElo, r.DRRank, r.DRTier, r.DRProgress, r.SRRating, r.SRRank, r.SRTier, r.SRProgress)
	return err == nil, err
}

const ratingColumns = `snapshot_id,player_id,observed_at,source,raw_path,dr_elo,dr_rank,dr_tier,dr_progress,sr_rating,sr_rank,sr_tier,sr_progress`

type scanner interface{ Scan(...any) error }

func scanRating(row scanner) (RatingSnapshot, error) {
	var r RatingSnapshot
	var at int64
	err := row.Scan(&r.ID, &r.PlayerID, &at, &r.Source, &r.RawPath, &r.DRElo, &r.DRRank, &r.DRTier, &r.DRProgress, &r.SRRating, &r.SRRank, &r.SRTier, &r.SRProgress)
	r.ObservedAt = time.UnixMicro(at).UTC()
	return r, err
}

// Ratings returns observations in [from,to), ordered by timestamp then ID.
func (s *Store) Ratings(ctx context.Context, steamID string, from, to time.Time) ([]RatingSnapshot, error) {
	if steamID == "" || !from.Before(to) {
		return nil, errors.New("Steam ID and an increasing time range are required")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+ratingColumns+` FROM rating_snapshots
 WHERE player_id=(SELECT player_id FROM players WHERE steam_id=?) AND observed_at>=? AND observed_at<? ORDER BY observed_at,snapshot_id`, steamID, ceilingMicro(from), ceilingMicro(to))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := []RatingSnapshot{}
	for rows.Next() {
		r, err := scanRating(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// Stored timestamps are integral microseconds. Ceiling both boundaries preserves
// [from,to) comparisons even when a consumer supplies nanosecond precision.
func ceilingMicro(at time.Time) int64 {
	value := at.UnixMicro()
	if at.Nanosecond()%1000 != 0 {
		value++
	}
	return value
}

// LatestRaces selects events before joining their sessions. References are
// resolved within the same read transaction, including observations at Event.At.
func (s *Store) LatestRaces(ctx context.Context, steamID string, limit int) ([]Race, error) {
	if steamID == "" || limit <= 0 {
		return nil, errors.New("Steam ID and a positive event limit are required")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT e.event_id,e.series_id,e.event_at,e.title,e.event_type,e.tier,e.track,e.track_scene,e.split_no,e.official,e.team_event,e.raw_path,e.imported_at,
 p.player_id,p.display_name,p.car_class,p.dr_rank,p.dr_tier,p.dr_progress,p.sr_rank,p.sr_tier,p.sr_progress,p.dr_adjustment,p.sr_adjustment,p.joker_played
 FROM events e JOIN event_participations p ON p.event_id=e.event_id
 WHERE p.player_id=(SELECT player_id FROM players WHERE steam_id=?)
 AND EXISTS(SELECT 1 FROM session_results r WHERE r.event_id=e.event_id AND r.player_id=p.player_id AND r.session_type='race')
 ORDER BY e.event_at DESC,e.event_id DESC LIMIT ?`, steamID, limit)
	if err != nil {
		return nil, err
	}
	races := []Race{}
	for rows.Next() {
		var r Race
		var at, imported int64
		e, p := &r.Event, &r.Participation
		err = rows.Scan(&e.ID, &e.SeriesID, &at, &e.Title, &e.Type, &e.Tier, &e.Track, &e.TrackScene, &e.SplitNo, &e.Official, &e.TeamEvent, &e.RawPath, &imported,
			&p.PlayerID, &p.Name, &p.CarClass, &p.DRRank, &p.DRTier, &p.DRProgress, &p.SRRank, &p.SRTier, &p.SRProgress, &p.DRAdjustment, &p.SRAdjustment, &p.JokerPlayed)
		if err != nil {
			rows.Close()
			return nil, err
		}
		e.At = time.UnixMicro(at).UTC()
		e.ImportedAt = time.UnixMicro(imported).UTC()
		p.EventID = e.ID
		races = append(races, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for i := range races {
		r := &races[i]
		r.Results, err = loadResults(ctx, tx, r.Event.ID, r.Participation.PlayerID)
		if err != nil {
			return nil, err
		}
		snapshot, err := scanRating(tx.QueryRowContext(ctx, `SELECT `+ratingColumns+` FROM rating_snapshots
   WHERE player_id=? AND observed_at<=? ORDER BY observed_at DESC,snapshot_id DESC LIMIT 1`, r.Participation.PlayerID, r.Event.At.UnixMicro()))
		if err == nil {
			r.ReferenceRating = &snapshot
		} else if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return races, nil
}

func loadResults(ctx context.Context, tx *sql.Tx, eventID string, playerID int64) ([]SessionResult, error) {
	rows, err := tx.QueryContext(ctx, `SELECT session_type,session_no,race_no,position,class_position,grid_position,class_grid_position,car_type,car_class,car_number,team_name,completed_laps,best_lap_us,finish_time_us,finish_status
 FROM session_results WHERE event_id=? AND player_id=? ORDER BY CASE session_type WHEN 'practice' THEN 1 WHEN 'qualifying' THEN 2 WHEN 'warmup' THEN 3 ELSE 4 END,session_no,race_no`, eventID, playerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := []SessionResult{}
	for rows.Next() {
		r := SessionResult{EventID: eventID, PlayerID: playerID}
		if err := rows.Scan(&r.SessionType, &r.SessionNo, &r.RaceNo, &r.Position, &r.ClassPosition, &r.GridPosition, &r.ClassGridPosition, &r.CarType, &r.CarClass, &r.CarNumber, &r.TeamName, &r.CompletedLaps, &r.BestLapUS, &r.FinishTimeUS, &r.FinishStatus); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}
