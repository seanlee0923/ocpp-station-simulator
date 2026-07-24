// Package webui embeds the built frontend (frontend/dist, copied here as
// dist/ by the root build script) into the Go binary, so a single compiled
// executable serves the whole app with no separate static file deployment
// step — see the portability goals in the project plan.
package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// FS returns the embedded frontend build with the "dist" prefix stripped,
// ready to serve at "/". Returns an error if the frontend hasn't been built
// yet (dist/ missing its index.html placeholder never happens in a real
// build; only relevant when running straight from source pre-build).
func FS() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}
