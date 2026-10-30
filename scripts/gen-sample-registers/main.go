package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	dir := "samples/check-registers"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir: %v\n", err)
		os.Exit(1)
	}
	for _, spec := range Specs() {
		body, err := Render(spec)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", spec.Filename, err)
			os.Exit(1)
		}
		path := filepath.Join(dir, spec.Filename)
		if err := os.WriteFile(path, body, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s (%d bytes)\n", path, len(body))
	}
}
