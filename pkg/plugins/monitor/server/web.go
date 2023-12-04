//go:build !dev

package server

import (
	"embed"
	"io/fs"
)

//go:embed web/dist
var monitorServerWebFS embed.FS

func (p *Plugin) webFS() fs.FS {
	f, _ := fs.Sub(monitorServerWebFS, "web/dist")
	return f
}
