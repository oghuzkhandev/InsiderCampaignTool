// Package migrations, goose migration dosyalarını binary'ye gömer.
package migrations

import "embed"

// FS, bu klasördeki .sql migration dosyalarını içerir.
//
//go:embed *.sql
var FS embed.FS
