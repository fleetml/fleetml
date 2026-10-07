// Package migrations embeds the SQL schema migrations so the server can apply
// them on start (MIGRATE_ON_START=true) without a separate migrate binary.
package migrations

import "embed"

// FS holds the *.up.sql / *.down.sql files in golang-migrate naming.
//
//go:embed *.sql
var FS embed.FS
