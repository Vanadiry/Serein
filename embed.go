package main

import "embed"

//go:embed all:web-dist
var webFiles embed.FS
