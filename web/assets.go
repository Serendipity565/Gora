package web

import "embed"

var (
	// StaticFiles contains the embedded web assets.
	//go:embed static/index.html
	StaticFiles embed.FS
)

// IndexHTML returns the embedded frontend entrypoint.
func IndexHTML() ([]byte, error) {
	return StaticFiles.ReadFile("static/index.html")
}
