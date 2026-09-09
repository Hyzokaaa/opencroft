package server

import "embed"

// The interface is compiled into the binary. `npm run build` in web/ writes here.
//
//go:embed all:dist
var assets embed.FS
