package server

import (
	"github.com/ncruces/zenity"
)

func pickDirectory(title string, startDir string) (string, error) {
	opts := []zenity.Option{
		zenity.Title(title),
		zenity.Directory(),
	}
	if startDir != "" {
		opts = append(opts, zenity.Filename(startDir))
	}
	return zenity.SelectFile(opts...)
}
