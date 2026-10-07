package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/rodaine/table"
)

// PrintTable renders data as an ASCII table.
func PrintTable(headers []string, rows [][]string) {
	if len(rows) == 0 {
		fmt.Println("No results.")
		return
	}
	ifaces := make([]interface{}, len(headers))
	for i, h := range headers {
		ifaces[i] = h
	}
	tbl := table.New(ifaces...)
	tbl.WithWriter(os.Stdout)
	for _, row := range rows {
		vals := make([]interface{}, len(row))
		for i, v := range row {
			vals[i] = v
		}
		tbl.AddRow(vals...)
	}
	tbl.Print()
}

// PrintJSON renders data as indented JSON.
func PrintJSON(data interface{}) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(data)
}

// PrintSuccess prints a success message.
func PrintSuccess(msg string) {
	fmt.Printf("✓ %s\n", msg)
}

// PrintError prints an error message.
func PrintError(msg string) {
	fmt.Fprintf(os.Stderr, "Error: %s\n", msg)
}

// PrintDetail prints a key-value detail block.
func PrintDetail(title string, pairs [][2]string) {
	fmt.Println(title)
	fmt.Println(strings.Repeat("─", len(title)+4))
	for _, p := range pairs {
		fmt.Printf("  %-20s %s\n", p[0]+":", p[1])
	}
}

// ProgressWriter wraps an io.Writer to show upload progress.
type ProgressWriter struct {
	Total   int64
	Written int64
}

func (pw *ProgressWriter) Write(p []byte) (int, error) {
	n := len(p)
	pw.Written += int64(n)
	pct := float64(pw.Written) / float64(pw.Total) * 100
	bar := int(pct / 2)
	fmt.Fprintf(os.Stderr, "\rUploading %s%s %.0f%% (%dKB/%dKB)",
		strings.Repeat("█", bar), strings.Repeat("░", 50-bar),
		pct, pw.Written/1024, pw.Total/1024)
	return n, nil
}
