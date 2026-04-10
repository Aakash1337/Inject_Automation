package main

import (
	"context"
	"fmt"
	"os"

	"injectctl/internal/cli"
)

var version = "dev"

func main() {
	if err := cli.Run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
