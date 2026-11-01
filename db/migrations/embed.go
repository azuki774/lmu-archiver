// Package migrations embeds the versioned archive schema into the CLI.
package migrations

import "embed"

// Files contains goose SQL migrations.
//
//go:embed *.sql
var Files embed.FS
