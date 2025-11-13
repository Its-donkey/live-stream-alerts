package main

import "testing"

func TestMainNotExecuted(t *testing.T) {
	t.Skip("main launches a long-running server; skipping runtime invocation")
}
