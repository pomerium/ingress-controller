//go:build embed_pomerium

package internal

import (
	"embed"
	"io/fs"

	"github.com/pomerium/pomerium/ui"
)

//go:embed ui/dist
var uiFS embed.FS

func init() {
	f, err := fs.Sub(uiFS, "ui")
	if err != nil {
		panic(err)
	}
	ui.ExtUIFS = f
}
