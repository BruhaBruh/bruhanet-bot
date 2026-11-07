// Package miniappweb — собранный фронтенд Mini App (vite → dist), вшитый в бинарь.
package miniappweb

import (
	"embed"
	"io/fs"
)

//go:embed dist
var dist embed.FS

// FS — содержимое dist без префикса.
func FS() fs.FS {
	sub, _ := fs.Sub(dist, "dist") // ошибка только при невалидном пути-литерале
	return sub
}
