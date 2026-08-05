package widgets

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/martino-vigiani/tiny/internal/spec"
)

func init() {
	Register(spec.TypeTable, tableWidget{})
}

type tableWidget struct{}

func (tableWidget) Run(ctx context.Context, s spec.Step, env *Env) (any, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	PrintHeader(env, s.Prompt())
	renderTable(env, s)
	if !s.Selectable {
		PrintNote(env, "Press enter to continue.")
		_, _, err := ReadInput(env)
		if err != nil {
			return nil, err
		}
		return nil, nil
	}
	suffix := "Select row (number), empty for default: "
	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		PrintPrompt(env, suffix)
		line, eof, err := ReadInput(env)
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			idx, err := tableDefaultIndex(s)
			if err != nil {
				if eof {
					if s.Required {
						return nil, unexpectedEOF(s.Key)
					}
					return nil, nil
				}
				PrintErrorf(env, "%v", err)
				continue
			}
			return tableAnswer(s, idx), nil
		}
		n, err := strconv.Atoi(line)
		if err != nil || n < 1 || n > len(s.Rows) {
			if eof {
				if s.Required {
					return nil, unexpectedEOF(s.Key)
				}
				return nil, nil
			}
			PrintErrorf(env, "Invalid row number, choose 1-%d.", len(s.Rows))
			continue
		}
		return tableAnswer(s, n-1), nil
	}
}

// renderTable draws the step's table. Column widths are the widest cell across
// the headers and every row in runes, plus two spaces of padding. When only
// headers are present, only the header row and separator are shown.
func renderTable(env *Env, s spec.Step) {
	numCols := len(s.Headers)
	for _, row := range s.Rows {
		if len(row) > numCols {
			numCols = len(row)
		}
	}
	widths := make([]int, numCols)
	for j := 0; j < numCols; j++ {
		w := 0
		if j < len(s.Headers) {
			w = utf8.RuneCountInString(s.Headers[j])
		}
		for _, row := range s.Rows {
			if j < len(row) && utf8.RuneCountInString(row[j]) > w {
				w = utf8.RuneCountInString(row[j])
			}
		}
		widths[j] = w + 2
	}
	t := env.look()
	if len(s.Headers) > 0 {
		env.buf = append(env.buf, t.TableHeaderOpen...)
		for j, h := range s.Headers {
			env.buf = appendPadded(env.buf, h, widths[j])
		}
		env.buf = append(env.buf, t.TableHeaderClose...)
		env.buf = append(env.buf, '\n')
		for j := 0; j < numCols; j++ {
			for w := 0; w < widths[j]; w++ {
				env.buf = append(env.buf, t.TableRule...)
			}
		}
		env.buf = append(env.buf, '\n')
	}
	for _, row := range s.Rows {
		for j := 0; j < numCols; j++ {
			cell := ""
			if j < len(row) {
				cell = row[j]
			}
			env.buf = appendPadded(env.buf, cell, widths[j])
		}
		env.buf = append(env.buf, '\n')
	}
	env.emit()
}

// appendPadded appends cell to dst, right-padded with spaces to width runes.
// It replaces the %-*s formatting the table used to do per cell.
func appendPadded(dst []byte, cell string, width int) []byte {
	dst = append(dst, cell...)
	for pad := width - utf8.RuneCountInString(cell); pad > 0; pad-- {
		dst = append(dst, ' ')
	}
	return dst
}

// tableDefaultIndex resolves the default for a selectable table: an integer
// is used as a 0-based row index, a string is matched case-insensitively
// against the first column.
func tableDefaultIndex(s spec.Step) (int, error) {
	switch d := s.Default.(type) {
	case int:
		if d >= 0 && d < len(s.Rows) {
			return d, nil
		}
		return 0, fmt.Errorf("Default row index %d out of range.", d)
	case int64:
		if d >= 0 && int(d) < len(s.Rows) {
			return int(d), nil
		}
		return 0, fmt.Errorf("Default row index %d out of range.", d)
	case float64:
		if d >= 0 && int(d) < len(s.Rows) {
			return int(d), nil
		}
		return 0, fmt.Errorf("Default row index %d out of range.", int(d))
	case string:
		for i, row := range s.Rows {
			if len(row) > 0 && strings.EqualFold(row[0], d) {
				return i, nil
			}
		}
		return 0, fmt.Errorf("Default row %q not found.", d)
	default:
		return 0, fmt.Errorf("Invalid default for selectable table.")
	}
}

// tableAnswer converts a chosen row into the step's answer: the row index by
// default, or that row's value in return_column when one is configured. The
// index is the honest default — a table's columns are presentation — but a
// caller who only wants the id should not have to index back into the rows it
// already wrote.
func tableAnswer(s spec.Step, row int) any {
	if s.ReturnColumn <= 0 {
		return row
	}
	if row < 0 || row >= len(s.Rows) {
		return row
	}
	cells := s.Rows[row]
	if s.ReturnColumn > len(cells) {
		return ""
	}
	return cells[s.ReturnColumn-1]
}
