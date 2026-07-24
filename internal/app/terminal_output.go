package app

import (
	"io"

	"github.com/pterm/pterm"
)

func printTerminalTable(out io.Writer, rows [][]string) error {
	if len(rows) == 0 {
		return nil
	}
	return pterm.DefaultTable.
		WithWriter(out).
		WithHasHeader().
		WithData(rows).
		Render()
}
