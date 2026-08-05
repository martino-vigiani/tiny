package widgets

import (
	"bufio"
	"context"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/martino-vigiani/tiny/internal/spec"
)

// benchEnv returns an Env writing to io.Discard, reading from r. It is the
// rendering-cost harness: the sink is free, so what a benchmark measures is the
// widget's own work.
func benchEnv(r *bufio.Reader) *Env {
	return &Env{
		In:      r,
		Out:     io.Discard,
		IsTTY:   false,
		StdinFd: 0,
	}
}

// benchOptions builds n option strings ("Option 01", ...).
func benchOptions(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "Option " + strconv.Itoa(i+1)
	}
	return out
}

// benchRows builds a rows×cols table body.
func benchRows(rows, cols int) [][]string {
	out := make([][]string, rows)
	for i := range out {
		row := make([]string, cols)
		for j := range row {
			row[j] = "cell-" + strconv.Itoa(i) + "-" + strconv.Itoa(j)
		}
		out[i] = row
	}
	return out
}

func BenchmarkPrintHeader(b *testing.B) {
	env := benchEnv(bufio.NewReader(strings.NewReader("")))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		PrintHeader(env, "Choose one")
	}
}

func BenchmarkPrintList20(b *testing.B) {
	env := benchEnv(bufio.NewReader(strings.NewReader("")))
	items := benchOptions(20)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		PrintList(env, items, 3)
	}
}

func BenchmarkPrintListMarked20(b *testing.B) {
	env := benchEnv(bufio.NewReader(strings.NewReader("")))
	items := benchOptions(20)
	marked := make([]bool, len(items))
	marked[3], marked[7], marked[11] = true, true, true
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		PrintListMarked(env, items, marked)
	}
}

func BenchmarkPrintPrompt(b *testing.B) {
	env := benchEnv(bufio.NewReader(strings.NewReader("")))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		PrintPrompt(env, "")
	}
}

func BenchmarkRenderTable(b *testing.B) {
	env := benchEnv(bufio.NewReader(strings.NewReader("")))
	step := spec.Step{
		Type:    spec.TypeTable,
		Key:     "rows",
		Title:   "Pick a row",
		Headers: []string{"Name", "Region", "Status", "Owner"},
		Rows:    benchRows(20, 4),
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		renderTable(env, step)
	}
}

func BenchmarkSelectRun(b *testing.B) {
	ctx := context.Background()
	step := spec.Step{
		Type:    spec.TypeSelect,
		Key:     "pick",
		Title:   "Choose one",
		Options: benchOptions(20),
	}
	sr := strings.NewReader("7\n")
	br := bufio.NewReader(sr)
	env := benchEnv(br)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sr.Reset("7\n")
		br.Reset(sr)
		if _, err := (selectWidget{}).Run(ctx, step, env); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMultiselectRun(b *testing.B) {
	ctx := context.Background()
	step := spec.Step{
		Type:    spec.TypeMultiselect,
		Key:     "pick",
		Title:   "Choose many",
		Options: benchOptions(20),
	}
	sr := strings.NewReader("1,4,9\n")
	br := bufio.NewReader(sr)
	env := benchEnv(br)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sr.Reset("1,4,9\n")
		br.Reset(sr)
		if _, err := (multiselectWidget{}).Run(ctx, step, env); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkMultiselectRunPreselected exercises the preselected() path: a
// multiselect with a default has to resolve it before drawing the list.
func BenchmarkMultiselectRunPreselected(b *testing.B) {
	ctx := context.Background()
	options := benchOptions(20)
	step := spec.Step{
		Type:    spec.TypeMultiselect,
		Key:     "pick",
		Title:   "Choose many",
		Options: options,
		Default: []string{options[1], options[5], options[12]},
	}
	sr := strings.NewReader("1,4,9\n")
	br := bufio.NewReader(sr)
	env := benchEnv(br)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sr.Reset("1,4,9\n")
		br.Reset(sr)
		if _, err := (multiselectWidget{}).Run(ctx, step, env); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkInputRun(b *testing.B) {
	ctx := context.Background()
	step := spec.Step{
		Type:        spec.TypeInput,
		Key:         "name",
		Title:       "Your name",
		Placeholder: "e.g. ada",
		Default:     "ada",
	}
	sr := strings.NewReader("grace\n")
	br := bufio.NewReader(sr)
	env := benchEnv(br)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sr.Reset("grace\n")
		br.Reset(sr)
		if _, err := (inputWidget{}).Run(ctx, step, env); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTableRun(b *testing.B) {
	ctx := context.Background()
	step := spec.Step{
		Type:       spec.TypeTable,
		Key:        "rows",
		Title:      "Pick a row",
		Headers:    []string{"Name", "Region", "Status", "Owner"},
		Rows:       benchRows(20, 4),
		Selectable: true,
	}
	sr := strings.NewReader("5\n")
	br := bufio.NewReader(sr)
	env := benchEnv(br)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sr.Reset("5\n")
		br.Reset(sr)
		if _, err := (tableWidget{}).Run(ctx, step, env); err != nil {
			b.Fatal(err)
		}
	}
}
