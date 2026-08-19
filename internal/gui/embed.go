package gui

import "embed"

//go:embed web/*
var webFS embed.FS

func ReadWeb(name string) ([]byte, error) {
	return webFS.ReadFile(name)
}
