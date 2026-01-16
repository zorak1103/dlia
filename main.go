// Package main is the entry point for the DLIA application.
package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/zorak1103/dlia/cmd"
)

func main() {
	// Panic recovery for production hardening. Catches unhandled panics and logs
	// the stack trace before terminating gracefully with exit code 1.
	// Exit code semantics: 0 = success, 1 = general error/panic, 2 = config error
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "\n❌ PANIC: %v\n", r)
			fmt.Fprintf(os.Stderr, "\nStack trace:\n%s\n", debug.Stack())
			os.Exit(1)
		}
	}()

	cmd.Execute()
}
