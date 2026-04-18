package botconfig

import (
	"embed"
	"fmt"
	"io/fs"

	tgamlconfig "github.com/shumdude/tgaml/pkg/config"
)

//go:embed config/*.yaml
var files embed.FS

func FS() (fs.FS, error) {
	sub, err := fs.Sub(files, "config")
	if err != nil {
		return nil, fmt.Errorf("sub config fs: %w", err)
	}
	return sub, nil
}

func Load() (*tgamlconfig.Config, error) {
	sub, err := FS()
	if err != nil {
		return nil, err
	}
	cfg, err := tgamlconfig.Load(sub)
	if err != nil {
		return nil, fmt.Errorf("load tgaml config: %w", err)
	}
	return cfg, nil
}
