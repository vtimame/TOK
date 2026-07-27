package app

import (
	"io"

	"github.com/pterm/pterm"
)

func printTerminalTable(out io.Writer, rows [][]string) error {
	if len(rows) == 0 {
		return nil
	}
	plainStyle := pterm.NewStyle()
	return pterm.DefaultTable.
		WithWriter(out).
		WithHasHeader().
		WithStyle(plainStyle).
		WithHeaderStyle(plainStyle).
		WithHeaderRowSeparatorStyle(plainStyle).
		WithSeparatorStyle(plainStyle).
		WithRowSeparatorStyle(plainStyle).
		WithData(rows).
		Render()
}
