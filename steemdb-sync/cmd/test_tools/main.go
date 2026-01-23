package main

import (
	"fmt"
	"os"
)

const usage = `test_tools - Testing utilities for steemdb-sync

Usage:
  test_tools <command> [flags]

Commands:
  jsonl_replay    Replay operations from JSONL file to ingest endpoint
  check_data      Validate MongoDB data integrity (blocks + operations)

Use "test_tools <command> -help" for command-specific help.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, usage)
		os.Exit(1)
	}

	command := os.Args[1]
	args := os.Args[2:]

	switch command {
	case "jsonl_replay":
		runJsonlReplay(args)
	case "check_data":
		runCheckData(args)
	case "help", "-h", "--help":
		fmt.Fprintf(os.Stderr, usage)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		fmt.Fprintf(os.Stderr, usage)
		os.Exit(1)
	}
}
