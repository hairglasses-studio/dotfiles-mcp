package dotfiles

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

func maybeRunAuxiliaryCommand(args []string) bool {
	if len(args) == 0 {
		return false
	}

	if args[0] == "--session-index" {
		if err := outputSessionIndex(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return true
	}

	if !containsContractFlag(args) {
		return false
	}

	fs := flag.NewFlagSet("dotfiles-mcp", flag.ExitOnError)
	profile := fs.String("contract-profile", "default", "DOTFILES_MCP_PROFILE to use when building the snapshot")
	write := fs.Bool("contract-write", false, "Write snapshots and .well-known/mcp.json into the repo")
	check := fs.Bool("contract-check", false, "Fail if committed snapshots or .well-known/mcp.json drift from generated output")
	printOverview := fs.Bool("contract-print", false, "Print the generated overview snapshot to stdout")
	_ = fs.Parse(args)

	bundle, err := BuildContractSnapshotBundle(*profile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "build contract snapshot: %v\n", err)
		os.Exit(1)
	}
	files, err := contractSnapshotFiles(bundle)
	if err != nil {
		fmt.Fprintf(os.Stderr, "render contract snapshot: %v\n", err)
		os.Exit(1)
	}

	switch {
	case *write:
		if err := writeContractSnapshotFiles(files); err != nil {
			fmt.Fprintf(os.Stderr, "write contract snapshot: %v\n", err)
			os.Exit(1)
		}
	case *check:
		if err := checkContractSnapshotFiles(files); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
	default:
		if !*printOverview {
			*printOverview = true
		}
		if *printOverview {
			out, err := SnapshotJSON(bundle.Overview)
			if err != nil {
				fmt.Fprintf(os.Stderr, "encode contract overview: %v\n", err)
				os.Exit(1)
			}
			_, _ = os.Stdout.Write(out)
			_, _ = os.Stdout.Write([]byte("\n"))
		}
	}

	return true
}

func containsContractFlag(args []string) bool {
	for _, arg := range args {
		switch {
		case arg == "--contract-write",
			arg == "--contract-check",
			arg == "--contract-print",
			arg == "--contract-profile",
			strings.HasPrefix(arg, "--contract-profile="):
			return true
		}
	}
	return false
}
