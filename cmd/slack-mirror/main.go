package main

import (
	"errors"
	"fmt"
	"os"
)

type command int

const (
	cmdNone command = iota
	cmdServe
	cmdBackfill
)

var errUsage = errors.New("usage: slack-mirror <serve|backfill>")

func parseCommand(args []string) (command, error) {
	if len(args) == 0 {
		return cmdNone, errUsage
	}
	switch args[0] {
	case "serve":
		return cmdServe, nil
	case "backfill":
		return cmdBackfill, nil
	default:
		return cmdNone, fmt.Errorf("unknown command %q: %w", args[0], errUsage)
	}
}

func main() {
	cmd, err := parseCommand(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	switch cmd {
	case cmdServe:
		fmt.Println("serve: not yet implemented") // replaced in a later task
	case cmdBackfill:
		fmt.Println("backfill: not yet implemented") // replaced in a later task
	}
}
