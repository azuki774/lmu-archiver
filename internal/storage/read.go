package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/azuki774/lmu-archiver/db/migrations"
)

// OpenReadOnly never creates files or applies migrations.
func OpenReadOnly(ctx context.Context, path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("database path is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, fmt.Errorf("open archive: %w; run sync or db migrate first", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("archive must be a regular database file")
	}
	u := url.URL{Scheme: "file", Path: absolute}
	q := url.Values{"mode": {"ro"}, "_pragma": {"foreign_keys(1)", "busy_timeout(5000)"}}
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	fail := func(err error) (*Store, error) {
		db.Close()
		return nil, fmt.Errorf("read archive: %w; run db migrate first", err)
	}
	var version int64
	if err := db.QueryRowContext(ctx, `SELECT version_id FROM goose_db_version WHERE is_applied=1 ORDER BY id DESC LIMIT 1`).Scan(&version); err != nil {
		return fail(err)
	}
	entries, err := migrations.Files.ReadDir(".")
	if err != nil {
		return fail(err)
	}
	var expected int64
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		prefix, _, _ := strings.Cut(entry.Name(), "_")
		value, err := strconv.ParseInt(prefix, 10, 64)
		if err != nil {
			return fail(err)
		}
		expected = max(expected, value)
	}
	if version != expected {
		return fail(errors.New("archive schema is not supported by this executable"))
	}
	return &Store{db: db}, nil
}

// ResolveSteamID returns the archive's sole tracked account. No account can be
// requested or assumed, so a shared archive is never read as one of its users.
func (s *Store) ResolveSteamID(ctx context.Context) (string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT steam_id FROM players ORDER BY player_id LIMIT 2`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return "", err
		}
		return "", ErrPlayerRequired
	}
	var id string
	if err := rows.Scan(&id); err != nil {
		return "", err
	}
	if rows.Next() {
		return "", ErrMultiplePlayers
	}
	return id, rows.Err()
}
