package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Belphemur/obsidian-headless/src-go/internal/cli"
)

func main() {
	application := cli.New(os.Stdin, os.Stdout, os.Stderr)
	if err := application.Execute(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
