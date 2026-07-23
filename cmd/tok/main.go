package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"s26.sh/tok/internal/app"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	cli := app.NewCLI(os.Stdout, os.Stderr, app.VersionInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
	})

	if err := cli.Run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)

		var usageErr *app.UsageError
		if errors.As(err, &usageErr) {
			os.Exit(usageErr.Code)
		}

		os.Exit(1)
	}
}
