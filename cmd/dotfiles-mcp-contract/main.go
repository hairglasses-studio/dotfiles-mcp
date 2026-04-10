package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	dotfiles "github.com/hairglasses-studio/dotfiles-mcp/internal/dotfiles"
)

func main() {
	var (
		profile = flag.String("profile", "default", "DOTFILES_MCP_PROFILE to use when building the snapshot")
		write   = flag.Bool("write", false, "Write snapshots and .well-known/mcp.json into the repo")
		check   = flag.Bool("check", false, "Fail if committed snapshots or .well-known/mcp.json drift from generated output")
	)
	flag.Parse()

	bundle, err := dotfiles.BuildContractSnapshotBundle(*profile)
	if err != nil {
		die("build snapshot: %v", err)
	}

	files, err := renderOutputs(bundle)
	if err != nil {
		die("render outputs: %v", err)
	}

	switch {
	case *write:
		for path, content := range files {
			if err := writeFile(path, content); err != nil {
				die("write %s: %v", path, err)
			}
		}
	case *check:
		var drift []string
		for path, content := range files {
			current, err := os.ReadFile(path)
			if err != nil {
				drift = append(drift, fmt.Sprintf("%s (missing: %v)", path, err))
				continue
			}
			if !bytes.Equal(current, content) {
				drift = append(drift, path)
			}
		}
		if len(drift) > 0 {
			die("contract drift detected:\n%s", joinLines(drift))
		}
	default:
		out, err := dotfiles.SnapshotJSON(bundle.Overview)
		if err != nil {
			die("encode overview: %v", err)
		}
		_, _ = os.Stdout.Write(out)
		_, _ = os.Stdout.Write([]byte("\n"))
	}
}

func renderOutputs(bundle dotfiles.ContractSnapshotBundle) (map[string][]byte, error) {
	files := map[string]any{
		".well-known/mcp.json":            bundle.Manifest,
		"snapshots/contract/overview.json":  bundle.Overview,
		"snapshots/contract/tools.json":     bundle.Tools,
		"snapshots/contract/resources.json": bundle.Resources,
		"snapshots/contract/templates.json": bundle.Templates,
		"snapshots/contract/prompts.json":   bundle.Prompts,
	}

	out := make(map[string][]byte, len(files))
	for path, payload := range files {
		content, err := dotfiles.SnapshotJSON(payload)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		content = append(content, '\n')
		out[path] = content
	}
	return out, nil
}

func writeFile(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func joinLines(items []string) string {
	var buf bytes.Buffer
	for _, item := range items {
		buf.WriteString(" - ")
		buf.WriteString(item)
		buf.WriteByte('\n')
	}
	return strings.TrimRight(buf.String(), "\n")
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
