package templates

import (
	"embed"
	"io/fs"
)

//go:embed assets manifest.json
var content embed.FS

func Load(name string) ([]byte, error) {
	if !fs.ValidPath(name) {
		return nil, fs.ErrInvalid
	}
	return content.ReadFile("assets/" + name)
}
