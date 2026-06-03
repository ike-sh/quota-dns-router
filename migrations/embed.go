package migrations

import "embed"

// FS exposes embedded SQL migration files.
//
//go:embed *.sql
var FS embed.FS
