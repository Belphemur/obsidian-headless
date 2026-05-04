package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/Belphemur/obsidian-headless/internal/cli"
	"github.com/spf13/cobra/doc"
)

func main() {
	outputDir := flag.String("output", "../website/src/cli-reference", "Output directory for generated markdown docs")
	flag.Parse()

	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create output directory: %v\n", err)
		os.Exit(1)
	}

	app := cli.New(nil, os.Stdout, os.Stderr)
	cmd := app.Command()
	if cmd == nil {
		fmt.Fprintln(os.Stderr, "Failed to get root command")
		os.Exit(1)
	}

	if err := doc.GenMarkdownTree(cmd, *outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to generate docs: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Generated CLI documentation in %s\n", *outputDir)
}
