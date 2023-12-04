//go:build dev

package server

import (
	"io/fs"
	"os"
)

func (p *Plugin) webFS() fs.FS {
	return os.DirFS("pkg/plugins/monitor/server/web/dist")
}
