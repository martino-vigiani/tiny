package runner_test

import (
	"io"
	"testing"

	"github.com/martino-vigiani/tiny/internal/runner"
)

// benchAnswers is a realistic answer map: one entry per canonical return type
// the widgets produce, so the serialisers hit every branch of writeEnvValue.
func benchAnswers() map[string]any {
	return map[string]any{
		"project.name":  "tiny",
		"region":        "eu-central-1",
		"features":      []string{"metrics", "tracing", "profiling"},
		"enable-tls":    true,
		"replicas":      float64(3),
		"created":       "2026-08-05",
		"password":      "s3cr3t p@ss",
		"labels":        map[string]string{"env": "prod", "team": "core", "tier": "1"},
		"notes":         "multi word value; with $pecials",
		"selected_row":  2,
		"editor.buffer": "line one\nline two",
	}
}

func BenchmarkPrintJSON(b *testing.B) {
	answers := benchAnswers()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := runner.PrintJSON(io.Discard, answers); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPrintEnv(b *testing.B) {
	answers := benchAnswers()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := runner.PrintEnv(io.Discard, answers); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPrintYAML(b *testing.B) {
	answers := benchAnswers()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := runner.PrintYAML(io.Discard, answers); err != nil {
			b.Fatal(err)
		}
	}
}
