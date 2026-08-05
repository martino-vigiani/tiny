package main

// bar: not implemented yet. The command lives here and its tests in bar_test.go.
func runBar(args []string, ui *UI) int {
	return usageError(ui, "bar is not implemented yet")
}
