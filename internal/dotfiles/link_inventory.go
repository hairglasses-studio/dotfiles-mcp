package dotfiles

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type linkSpec struct {
	src string
	dst string
}

func linkSpecsScriptPath() string {
	return filepath.Join(dotfilesDir(), "scripts", "link-specs.sh")
}

// installerScriptPath is kept as a fallback for environments without chezmoi.
func installerScriptPath() string {
	return filepath.Join(dotfilesDir(), "install.sh")
}

func parseLinkSpecsOutput(raw string) ([]linkSpec, error) {
	scanner := bufio.NewScanner(strings.NewReader(raw))
	var links []linkSpec
	lineNo := 0

	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("invalid link spec line %d: %q", lineNo, line)
		}
		links = append(links, linkSpec{src: parts[0], dst: parts[1]})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan install inventory: %w", err)
	}
	if len(links) == 0 {
		return nil, fmt.Errorf("installer returned no link specs")
	}
	return links, nil
}

func loadManagedLinkSpecs() ([]linkSpec, error) {
	// Prefer the chezmoi-based link-specs.sh; fall back to install.sh --print-link-specs
	script := linkSpecsScriptPath()
	args := []string{script}
	if _, err := os.Stat(script); err != nil {
		script = installerScriptPath()
		args = []string{script, "--print-link-specs"}
		if _, err2 := os.Stat(script); err2 != nil {
			return nil, fmt.Errorf("no link-specs script available (tried scripts/link-specs.sh and install.sh): %w", err)
		}
	}

	cmd := exec.Command("bash", args...)
	cmd.Env = os.Environ()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return nil, fmt.Errorf("link-specs failed: %w: %s", err, msg)
		}
		return nil, fmt.Errorf("link-specs failed: %w", err)
	}

	links, err := parseLinkSpecsOutput(stdout.String())
	if err != nil {
		return nil, fmt.Errorf("parse link inventory: %w", err)
	}
	return links, nil
}
