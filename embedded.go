package main

import (
	"embed"
	"io/fs"
)

//go:embed all:frontend/dist
var frontendDist embed.FS

func getFrontendFS() fs.FS {
	sub, err := fs.Sub(frontendDist, "frontend/dist")
	if err != nil {
		return nil
	}
	return sub
}
