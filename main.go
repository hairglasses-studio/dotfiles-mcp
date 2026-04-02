// Command dotfiles-mcp is an MCP server for dotfiles configuration management.
//
// It provides validated config editing, symlink health checks, and service
// reloading over the Model Context Protocol (stdio transport).
//
// Usage:
//
//	DOTFILES_DIR=$HOME/hairglasses-studio/dotfiles dotfiles-mcp
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func dotfilesDir() string {
	if d := os.Getenv("DOTFILES_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "hairglasses-studio", "dotfiles")
}

func homeDir() string {
	h, _ := os.UserHomeDir()
	return h
}

func jsonResult(v any) (*mcp.CallToolResult, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("json marshal: %v", err)), nil
	}
	return mcp.NewToolResultText(string(b)), nil
}

func errResult(msg string) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultError(msg), nil
}

// detectFormat guesses a config format from the file extension.
func detectFormat(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".toml":
		return "toml"
	case ".json", ".jsonc":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".ini", ".conf":
		return "ini"
	default:
		return "unknown"
	}
}

// ---------------------------------------------------------------------------
// Tool 1: dotfiles_list_configs
// ---------------------------------------------------------------------------

type configEntry struct {
	Name           string `json:"name"`
	Path           string `json:"path"`
	SymlinkTarget  string `json:"symlink_target,omitempty"`
	SymlinkHealthy *bool  `json:"symlink_healthy,omitempty"`
	Format         string `json:"format"`
}

func listConfigs(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	dir := dotfilesDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return errResult(fmt.Sprintf("read dotfiles dir %s: %v", dir, err))
	}

	home := homeDir()
	configDir := filepath.Join(home, ".config")

	var configs []configEntry
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		// Skip hidden directories (.git, etc.)
		if strings.HasPrefix(name, ".") {
			continue
		}
		srcPath := filepath.Join(dir, name)

		ce := configEntry{
			Name: name,
			Path: srcPath,
		}

		// Detect format from the most prominent config file in the directory.
		subEntries, _ := os.ReadDir(srcPath)
		for _, se := range subEntries {
			if se.IsDir() {
				continue
			}
			f := detectFormat(se.Name())
			if f != "unknown" {
				ce.Format = f
				break
			}
		}
		if ce.Format == "" {
			ce.Format = "unknown"
		}

		// Check if a symlink exists in ~/.config/ pointing here.
		symlinkPath := filepath.Join(configDir, name)
		if target, lerr := os.Readlink(symlinkPath); lerr == nil {
			ce.SymlinkTarget = target
			healthy := target == srcPath
			ce.SymlinkHealthy = &healthy
		}

		configs = append(configs, ce)
	}

	return jsonResult(configs)
}

// ---------------------------------------------------------------------------
// Tool 2: dotfiles_validate_config
// ---------------------------------------------------------------------------

type validationResult struct {
	Valid bool   `json:"valid"`
	Error string `json:"error,omitempty"`
	Line  int    `json:"line,omitempty"`
}

func validateConfig(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	content, _ := args["content"].(string)
	format, _ := args["format"].(string)

	if content == "" {
		return errResult("content is required")
	}
	if format == "" {
		return errResult("format is required (toml, json)")
	}

	vr := validationResult{Valid: true}

	switch strings.ToLower(format) {
	case "toml":
		_, err := toml.NewDecoder(strings.NewReader(content)).Decode(new(map[string]any))
		if err != nil {
			vr.Valid = false
			vr.Error = err.Error()
			// Attempt to extract line number from TOML parse error.
			if pe, ok := err.(toml.ParseError); ok {
				msg := pe.Error()
				var line int
				if _, serr := fmt.Sscanf(msg, "toml: line %d", &line); serr == nil {
					vr.Line = line
				}
			}
		}

	case "json":
		var dst any
		dec := json.NewDecoder(strings.NewReader(content))
		if err := dec.Decode(&dst); err != nil {
			vr.Valid = false
			vr.Error = err.Error()
			if se, ok := err.(*json.SyntaxError); ok {
				// Count newlines to approximate line number.
				line := 1 + strings.Count(content[:se.Offset], "\n")
				vr.Line = line
			}
		}

	default:
		return errResult(fmt.Sprintf("unsupported format %q: supported formats are toml, json", format))
	}

	return jsonResult(vr)
}

// ---------------------------------------------------------------------------
// Tool 3: dotfiles_reload_service
// ---------------------------------------------------------------------------

type reloadResult struct {
	Service string `json:"service"`
	Success bool   `json:"success"`
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
}

// reloadCommands maps service name -> reload command.
var reloadCommands = map[string][]string{
	"hyprland": {"hyprctl", "reload"},
	"mako":     {"makoctl", "reload"},
	"eww":      {"eww", "reload"},
	"waybar":   {"pkill", "-SIGUSR2", "waybar"},
	"sway":     {"swaymsg", "reload"},
	"tmux":     {"tmux", "source-file", "~/.tmux.conf"},
}

func reloadService(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	svc, _ := args["service"].(string)
	if svc == "" {
		return errResult("service is required")
	}

	cmdParts, ok := reloadCommands[strings.ToLower(svc)]
	if !ok {
		supported := make([]string, 0, len(reloadCommands))
		for k := range reloadCommands {
			supported = append(supported, k)
		}
		return errResult(fmt.Sprintf("unknown service %q; supported: %s", svc, strings.Join(supported, ", ")))
	}

	cmd := exec.Command(cmdParts[0], cmdParts[1:]...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	rr := reloadResult{Service: svc}
	if err := cmd.Run(); err != nil {
		rr.Success = false
		rr.Error = fmt.Sprintf("%v: %s", err, strings.TrimSpace(stderr.String()))
	} else {
		rr.Success = true
	}
	out := strings.TrimSpace(stdout.String())
	if out != "" {
		rr.Output = out
	}

	return jsonResult(rr)
}

// ---------------------------------------------------------------------------
// Tool 4: dotfiles_check_symlinks
// ---------------------------------------------------------------------------

type symlinkStatus struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Status string `json:"status"` // healthy, broken, missing
	Actual string `json:"actual,omitempty"`
}

// expectedSymlinks returns the symlink mapping from install.sh logic.
func expectedSymlinks() []struct{ src, dst string } {
	dir := dotfilesDir()
	home := homeDir()
	cfg := filepath.Join(home, ".config")

	// Common (all platforms).
	links := []struct{ src, dst string }{
		{filepath.Join(dir, "zsh/zshrc"), filepath.Join(home, ".zshrc")},
		{filepath.Join(dir, "zsh/p10k.zsh"), filepath.Join(home, ".p10k.zsh")},
		{filepath.Join(dir, "zsh/zshenv"), filepath.Join(home, ".zshenv")},
		{filepath.Join(dir, "git/gitconfig"), filepath.Join(home, ".gitconfig")},
		{filepath.Join(dir, "ssh/config"), filepath.Join(home, ".ssh/config")},
		{filepath.Join(dir, "starship/starship.toml"), filepath.Join(cfg, "starship.toml")},
		{filepath.Join(dir, "ghostty"), filepath.Join(cfg, "ghostty")},
		{filepath.Join(dir, "nvim"), filepath.Join(cfg, "nvim")},
		{filepath.Join(dir, "bat"), filepath.Join(cfg, "bat")},
		{filepath.Join(dir, "fastfetch"), filepath.Join(cfg, "fastfetch")},
		{filepath.Join(dir, "git/delta"), filepath.Join(cfg, "delta")},
		{filepath.Join(dir, "git/ignore"), filepath.Join(cfg, "git/ignore")},
		{filepath.Join(dir, "gh"), filepath.Join(cfg, "gh")},
		{filepath.Join(dir, "k9s"), filepath.Join(cfg, "k9s")},
		{filepath.Join(dir, "lazygit"), filepath.Join(cfg, "lazygit")},
		{filepath.Join(dir, "btop"), filepath.Join(cfg, "btop")},
		{filepath.Join(dir, "yazi"), filepath.Join(cfg, "yazi")},
		{filepath.Join(dir, "cava"), filepath.Join(cfg, "cava")},
		{filepath.Join(dir, "glow"), filepath.Join(cfg, "glow")},
		{filepath.Join(dir, "tmux/tmux.conf"), filepath.Join(home, ".tmux.conf")},
	}

	// Platform-specific links.
	if runtime.GOOS == "darwin" {
		links = append(links,
			struct{ src, dst string }{filepath.Join(dir, "aerospace/aerospace.toml"), filepath.Join(home, ".aerospace.toml")},
			struct{ src, dst string }{filepath.Join(dir, "sketchybar"), filepath.Join(cfg, "sketchybar")},
			struct{ src, dst string }{filepath.Join(dir, "borders"), filepath.Join(cfg, "borders")},
			struct{ src, dst string }{filepath.Join(dir, "tattoo/tattoy.toml"), filepath.Join(home, "Library/Application Support/tattoy/tattoy.toml")},
		)
	} else if runtime.GOOS == "linux" {
		links = append(links,
			struct{ src, dst string }{filepath.Join(dir, "sway/config"), filepath.Join(cfg, "sway/config")},
			struct{ src, dst string }{filepath.Join(dir, "waybar/config.jsonc"), filepath.Join(cfg, "waybar/config")},
			struct{ src, dst string }{filepath.Join(dir, "waybar/style.css"), filepath.Join(cfg, "waybar/style.css")},
			struct{ src, dst string }{filepath.Join(dir, "mako/config"), filepath.Join(cfg, "mako/config")},
			struct{ src, dst string }{filepath.Join(dir, "wofi/config"), filepath.Join(cfg, "wofi/config")},
			struct{ src, dst string }{filepath.Join(dir, "wofi/style.css"), filepath.Join(cfg, "wofi/style.css")},
			struct{ src, dst string }{filepath.Join(dir, "foot/foot.ini"), filepath.Join(cfg, "foot/foot.ini")},
			struct{ src, dst string }{filepath.Join(dir, "hyprland"), filepath.Join(cfg, "hypr")},
			struct{ src, dst string }{filepath.Join(dir, "eww"), filepath.Join(cfg, "eww")},
			struct{ src, dst string }{filepath.Join(dir, "tattoy/tattoy.toml"), filepath.Join(cfg, "tattoy/tattoy.toml")},
		)
	}

	return links
}

func checkSymlinks(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	links := expectedSymlinks()
	var results []symlinkStatus

	for _, l := range links {
		ss := symlinkStatus{
			Source: l.src,
			Target: l.dst,
		}

		target, err := os.Readlink(l.dst)
		if err != nil {
			if os.IsNotExist(err) {
				ss.Status = "missing"
			} else {
				ss.Status = "missing"
			}
		} else if target == l.src {
			ss.Status = "healthy"
		} else {
			ss.Status = "broken"
			ss.Actual = target
		}

		results = append(results, ss)
	}

	return jsonResult(results)
}

// ---------------------------------------------------------------------------
// Tool 5: dotfiles_gh_list_personal_repos
// ---------------------------------------------------------------------------

type repoInfo struct {
	Name       string `json:"name"`
	FullName   string `json:"full_name"`
	Private    bool   `json:"private"`
	Fork       bool   `json:"fork"`
	Language   string `json:"language,omitempty"`
	Branch     string `json:"default_branch"`
	Archived   bool   `json:"archived"`
	Stars      int    `json:"stargazers_count"`
	Forks      int    `json:"forks_count"`
	Description string `json:"description,omitempty"`
}

func ghListPersonalRepos(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	owner, _ := args["owner"].(string)
	excludeOrg, _ := args["exclude_org"].(string)

	if owner == "" {
		return errResult("owner is required")
	}

	// Fetch all repos for the user via gh CLI
	cmd := exec.Command("gh", "api", "--paginate",
		fmt.Sprintf("/users/%s/repos?per_page=100&type=owner&sort=updated", owner),
		"--jq", ".[] | {name, full_name, private, fork, language, default_branch, archived, stargazers_count, forks_count, description}")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return errResult(fmt.Sprintf("gh api failed: %v: %s", err, strings.TrimSpace(stderr.String())))
	}

	// Parse newline-delimited JSON objects
	var repos []repoInfo
	dec := json.NewDecoder(strings.NewReader(stdout.String()))
	for dec.More() {
		var r repoInfo
		if err := dec.Decode(&r); err != nil {
			continue
		}
		// Filter out repos already in the target org
		if excludeOrg != "" {
			orgPrefix := excludeOrg + "/"
			if strings.HasPrefix(r.FullName, orgPrefix) {
				continue
			}
		}
		repos = append(repos, r)
	}

	type summary struct {
		Total    int        `json:"total"`
		Originals int      `json:"originals"`
		Forks    int        `json:"forks"`
		Repos    []repoInfo `json:"repos"`
	}

	s := summary{Total: len(repos), Repos: repos}
	for _, r := range repos {
		if r.Fork {
			s.Forks++
		} else {
			s.Originals++
		}
	}

	return jsonResult(s)
}

// ---------------------------------------------------------------------------
// Tool 6: dotfiles_gh_transfer_repos
// ---------------------------------------------------------------------------

type transferResult struct {
	Repo    string `json:"repo"`
	Action  string `json:"action"` // transferred, skipped, failed
	Message string `json:"message,omitempty"`
}

func ghTransferRepos(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	owner, _ := args["owner"].(string)
	targetOrg, _ := args["target_org"].(string)
	dryRun := true
	if v, ok := args["execute"].(bool); ok {
		dryRun = !v
	}

	if owner == "" {
		return errResult("owner is required")
	}
	if targetOrg == "" {
		return errResult("target_org is required")
	}

	// Parse repos array
	repoNames, err := parseStringArray(args["repos"])
	if err != nil || len(repoNames) == 0 {
		return errResult("repos is required (array of repo names)")
	}

	var results []transferResult

	for _, repo := range repoNames {
		// Check if already in target org
		checkCmd := exec.Command("gh", "api", fmt.Sprintf("repos/%s/%s", targetOrg, repo), "--jq", ".full_name")
		if checkCmd.Run() == nil {
			results = append(results, transferResult{Repo: repo, Action: "skipped", Message: "already exists in " + targetOrg})
			continue
		}

		// Verify source exists and is not a fork
		metaCmd := exec.Command("gh", "api", fmt.Sprintf("repos/%s/%s", owner, repo), "--jq", ".fork")
		var metaOut bytes.Buffer
		metaCmd.Stdout = &metaOut
		if err := metaCmd.Run(); err != nil {
			results = append(results, transferResult{Repo: repo, Action: "failed", Message: "source not found"})
			continue
		}
		if strings.TrimSpace(metaOut.String()) == "true" {
			results = append(results, transferResult{Repo: repo, Action: "failed", Message: "is a fork — use dotfiles_gh_recreate_forks instead"})
			continue
		}

		if dryRun {
			results = append(results, transferResult{Repo: repo, Action: "dry-run", Message: "would transfer to " + targetOrg})
			continue
		}

		// Transfer
		transferCmd := exec.Command("gh", "api", "--method", "POST",
			fmt.Sprintf("repos/%s/%s/transfer", owner, repo),
			"-f", "new_owner="+targetOrg, "--silent")
		var transferErr bytes.Buffer
		transferCmd.Stderr = &transferErr
		if err := transferCmd.Run(); err != nil {
			results = append(results, transferResult{Repo: repo, Action: "failed", Message: strings.TrimSpace(transferErr.String())})
		} else {
			results = append(results, transferResult{Repo: repo, Action: "transferred", Message: "transferred to " + targetOrg})
		}
	}

	return jsonResult(results)
}

// ---------------------------------------------------------------------------
// Tool 7: dotfiles_gh_recreate_forks
// ---------------------------------------------------------------------------

func ghRecreateForks(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	owner, _ := args["owner"].(string)
	targetOrg, _ := args["target_org"].(string)
	dryRun := true
	if v, ok := args["execute"].(bool); ok {
		dryRun = !v
	}

	if owner == "" {
		return errResult("owner is required")
	}
	if targetOrg == "" {
		return errResult("target_org is required")
	}

	repoNames, err := parseStringArray(args["repos"])
	if err != nil || len(repoNames) == 0 {
		return errResult("repos is required (array of repo names)")
	}

	var results []transferResult

	for _, repo := range repoNames {
		// Check if already in target org
		checkCmd := exec.Command("gh", "api", fmt.Sprintf("repos/%s/%s", targetOrg, repo), "--jq", ".full_name")
		if checkCmd.Run() == nil {
			results = append(results, transferResult{Repo: repo, Action: "skipped", Message: "already exists in " + targetOrg})
			continue
		}

		// Get metadata
		metaCmd := exec.Command("gh", "api", fmt.Sprintf("repos/%s/%s", owner, repo),
			"--jq", "{default_branch: .default_branch, private: .private, description: .description}")
		var metaOut bytes.Buffer
		metaCmd.Stdout = &metaOut
		if err := metaCmd.Run(); err != nil {
			results = append(results, transferResult{Repo: repo, Action: "failed", Message: "could not fetch metadata"})
			continue
		}

		var meta struct {
			DefaultBranch string `json:"default_branch"`
			Private       bool   `json:"private"`
			Description   string `json:"description"`
		}
		if err := json.Unmarshal(metaOut.Bytes(), &meta); err != nil {
			results = append(results, transferResult{Repo: repo, Action: "failed", Message: "bad metadata: " + err.Error()})
			continue
		}

		visibility := "public"
		if meta.Private {
			visibility = "private"
		}

		if dryRun {
			results = append(results, transferResult{
				Repo:    repo,
				Action:  "dry-run",
				Message: fmt.Sprintf("would clone, squash to 1 commit on %s, create %s/%s (%s), delete fork", meta.DefaultBranch, targetOrg, repo, visibility),
			})
			continue
		}

		// Create temp dir for this repo
		tmpDir, err := os.MkdirTemp("", "fork-recreate-"+repo+"-")
		if err != nil {
			results = append(results, transferResult{Repo: repo, Action: "failed", Message: "mkdtemp: " + err.Error()})
			continue
		}

		// Clone
		cloneCmd := exec.Command("git", "clone", "--quiet",
			fmt.Sprintf("https://github.com/%s/%s.git", owner, repo), tmpDir)
		if out, err := cloneCmd.CombinedOutput(); err != nil {
			os.RemoveAll(tmpDir)
			results = append(results, transferResult{Repo: repo, Action: "failed", Message: "clone failed: " + strings.TrimSpace(string(out))})
			continue
		}

		// Checkout default branch
		runGit := func(args ...string) (string, error) {
			c := exec.Command("git", args...)
			c.Dir = tmpDir
			out, err := c.CombinedOutput()
			return strings.TrimSpace(string(out)), err
		}

		runGit("checkout", meta.DefaultBranch)

		// Squash: orphan branch with all files from HEAD
		if _, err := runGit("checkout", "--orphan", "squashed"); err != nil {
			os.RemoveAll(tmpDir)
			results = append(results, transferResult{Repo: repo, Action: "failed", Message: "orphan checkout failed"})
			continue
		}
		runGit("add", "-A")
		if _, err := runGit("commit", "-m", fmt.Sprintf("Initial commit (migrated from %s/%s)", owner, repo)); err != nil {
			os.RemoveAll(tmpDir)
			results = append(results, transferResult{Repo: repo, Action: "failed", Message: "squash commit failed (empty repo?)"})
			continue
		}
		runGit("branch", "-M", meta.DefaultBranch)

		// Create repo in org and push
		createArgs := []string{"repo", "create", fmt.Sprintf("%s/%s", targetOrg, repo), "--" + visibility, "--source", tmpDir, "--push"}
		if meta.Description != "" {
			createArgs = append(createArgs[:5], append([]string{"--description", meta.Description}, createArgs[5:]...)...)
		}
		createCmd := exec.Command("gh", createArgs...)
		if out, err := createCmd.CombinedOutput(); err != nil {
			os.RemoveAll(tmpDir)
			results = append(results, transferResult{Repo: repo, Action: "failed", Message: "create/push failed: " + strings.TrimSpace(string(out))})
			continue
		}

		os.RemoveAll(tmpDir)

		// Delete original fork
		delCmd := exec.Command("gh", "api", "--method", "DELETE", fmt.Sprintf("repos/%s/%s", owner, repo), "--silent")
		if err := delCmd.Run(); err != nil {
			results = append(results, transferResult{Repo: repo, Action: "recreated", Message: "created in " + targetOrg + " but failed to delete original fork"})
		} else {
			results = append(results, transferResult{Repo: repo, Action: "recreated", Message: "squashed and recreated in " + targetOrg + ", fork deleted"})
		}
	}

	return jsonResult(results)
}

// ---------------------------------------------------------------------------
// Tool 8: dotfiles_eww_restart
// ---------------------------------------------------------------------------

type ewwRestartResult struct {
	Killed    int    `json:"killed"`
	WaybarOff bool   `json:"waybar_killed"`
	DaemonPID string `json:"daemon_pid,omitempty"`
	BarsOpen  []string `json:"bars_opened"`
	Error     string `json:"error,omitempty"`
}

func ewwRestart(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	r := ewwRestartResult{}

	// Kill waybar if running (legacy cleanup)
	waybar := exec.Command("killall", "waybar")
	if waybar.Run() == nil {
		r.WaybarOff = true
	}

	// Count and kill existing eww processes
	countCmd := exec.Command("pgrep", "-c", "eww")
	var countOut bytes.Buffer
	countCmd.Stdout = &countOut
	if countCmd.Run() == nil {
		fmt.Sscanf(strings.TrimSpace(countOut.String()), "%d", &r.Killed)
	}

	exec.Command("killall", "-9", "eww").Run()

	// Wait for processes to die
	exec.Command("sleep", "1").Run()

	// Remove stale socket
	home := homeDir()
	sockets, _ := filepath.Glob(filepath.Join("/run/user/1000", "eww-server_*"))
	for _, s := range sockets {
		os.Remove(s)
	}

	// Start daemon
	daemon := exec.Command("eww", "daemon", "--restart")
	daemon.Dir = home
	if err := daemon.Start(); err != nil {
		r.Error = fmt.Sprintf("daemon start failed: %v", err)
		return jsonResult(r)
	}

	// Wait for daemon to initialize
	exec.Command("sleep", "3").Run()

	// Verify daemon is responsive
	ping := exec.Command("eww", "ping")
	var pingOut bytes.Buffer
	ping.Stdout = &pingOut
	if err := ping.Run(); err != nil {
		r.Error = "daemon started but not responding to ping"
		return jsonResult(r)
	}

	// Get daemon PID
	pidCmd := exec.Command("pgrep", "-o", "eww")
	var pidOut bytes.Buffer
	pidCmd.Stdout = &pidOut
	if pidCmd.Run() == nil {
		r.DaemonPID = strings.TrimSpace(pidOut.String())
	}

	// Open bars
	bars := []string{"bar", "bar-secondary"}
	for _, bar := range bars {
		openCmd := exec.Command("eww", "open", bar)
		if err := openCmd.Run(); err == nil {
			r.BarsOpen = append(r.BarsOpen, bar)
		}
	}

	return jsonResult(r)
}

// ---------------------------------------------------------------------------
// Tool 9: dotfiles_eww_status
// ---------------------------------------------------------------------------

type ewwBarStatus struct {
	DaemonRunning bool              `json:"daemon_running"`
	DaemonCount   int               `json:"daemon_count"`
	WaybarRunning bool              `json:"waybar_running"`
	Windows       []string          `json:"windows"`
	Layers        []ewwLayerInfo    `json:"layers"`
	Variables     map[string]string `json:"variables,omitempty"`
}

type ewwLayerInfo struct {
	Monitor   string `json:"monitor"`
	Namespace string `json:"namespace"`
	Position  string `json:"position"`
}

func ewwStatus(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	st := ewwBarStatus{
		Variables: make(map[string]string),
	}

	// Check eww daemon count
	countCmd := exec.Command("pgrep", "-c", "eww")
	var countOut bytes.Buffer
	countCmd.Stdout = &countOut
	if countCmd.Run() == nil {
		fmt.Sscanf(strings.TrimSpace(countOut.String()), "%d", &st.DaemonCount)
		st.DaemonRunning = st.DaemonCount > 0
	}

	// Check waybar
	waybarCmd := exec.Command("pgrep", "-x", "waybar")
	st.WaybarRunning = waybarCmd.Run() == nil

	// List open eww windows
	winCmd := exec.Command("eww", "active-windows")
	var winOut bytes.Buffer
	winCmd.Stdout = &winOut
	if winCmd.Run() == nil {
		for _, line := range strings.Split(strings.TrimSpace(winOut.String()), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			// Format: "bar: bar" — extract the window name (before colon)
			if parts := strings.SplitN(line, ":", 2); len(parts) > 0 {
				st.Windows = append(st.Windows, strings.TrimSpace(parts[0]))
			}
		}
	}

	// Get layer info from hyprctl
	layerCmd := exec.Command("hyprctl", "layers")
	var layerOut bytes.Buffer
	layerCmd.Stdout = &layerOut
	if layerCmd.Run() == nil {
		var currentMonitor string
		for _, line := range strings.Split(layerOut.String(), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "Monitor ") {
				currentMonitor = strings.TrimSuffix(strings.TrimPrefix(line, "Monitor "), ":")
			}
			if strings.Contains(line, "namespace:") {
				parts := strings.Split(line, "namespace: ")
				if len(parts) == 2 {
					ns := strings.Split(parts[1], ",")[0]
					xywh := ""
					if xywhIdx := strings.Index(line, "xywh: "); xywhIdx >= 0 {
						xywh = strings.Split(line[xywhIdx+6:], ",")[0]
					}
					st.Layers = append(st.Layers, ewwLayerInfo{
						Monitor:   currentMonitor,
						Namespace: ns,
						Position:  xywh,
					})
				}
			}
		}
	}

	// Get key eww variable values
	varsToCheck := []string{
		"bar_workspaces_dp1", "bar_workspaces_dp2",
		"bar_cpu", "bar_mem", "bar_vol", "bar_shader",
	}
	for _, v := range varsToCheck {
		getCmd := exec.Command("eww", "get", v)
		var getOut bytes.Buffer
		getCmd.Stdout = &getOut
		if getCmd.Run() == nil {
			st.Variables[v] = strings.TrimSpace(getOut.String())
		}
	}

	return jsonResult(st)
}

// ---------------------------------------------------------------------------
// Tool 10: dotfiles_eww_get
// ---------------------------------------------------------------------------

func ewwGet(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	varName, _ := args["variable"].(string)

	if varName == "" {
		return errResult("variable name is required")
	}

	cmd := exec.Command("eww", "get", varName)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return errResult(fmt.Sprintf("eww get %s failed: %v: %s", varName, err, strings.TrimSpace(stderr.String())))
	}

	value := strings.TrimSpace(stdout.String())

	// Try to parse as JSON for structured output
	var parsed any
	if json.Unmarshal([]byte(value), &parsed) == nil {
		result := map[string]any{
			"variable": varName,
			"value":    parsed,
		}
		return jsonResult(result)
	}

	return jsonResult(map[string]string{
		"variable": varName,
		"value":    value,
	})
}

// parseStringArray extracts a []string from an interface{} that may be []any.
func parseStringArray(v any) ([]string, error) {
	if v == nil {
		return nil, fmt.Errorf("nil value")
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("not an array")
	}
	var out []string
	for _, item := range arr {
		s, ok := item.(string)
		if !ok {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Tool 11: dotfiles_onboard_repo
// ---------------------------------------------------------------------------

func onboardRepo(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	repoPath, _ := args["repo_path"].(string)
	lang, _ := args["language"].(string)
	dryRun, _ := args["dry_run"].(bool)

	if repoPath == "" {
		return errResult("repo_path is required")
	}

	scriptArgs := []string{repoPath}
	if lang != "" {
		scriptArgs = append(scriptArgs, "--language="+lang)
	}
	if dryRun {
		scriptArgs = append(scriptArgs, "--dry-run")
	}

	script := filepath.Join(dotfilesDir(), "scripts", "hg-onboard-repo.sh")
	cmd := exec.Command(script, scriptArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return jsonResult(map[string]any{
		"repo":   filepath.Base(repoPath),
		"status": map[bool]string{true: "fail", false: "ok"}[err != nil],
		"output": stdout.String(),
		"error":  stderr.String(),
	})
}

// ---------------------------------------------------------------------------
// Tool 12: dotfiles_pipeline_run
// ---------------------------------------------------------------------------

func pipelineRun(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	repoPath, _ := args["repo_path"].(string)
	buildOnly, _ := args["build_only"].(bool)
	testOnly, _ := args["test_only"].(bool)

	if repoPath == "" {
		return errResult("repo_path is required")
	}

	scriptArgs := []string{repoPath, "--json"}
	if buildOnly {
		scriptArgs = append(scriptArgs, "--build-only")
	}
	if testOnly {
		scriptArgs = append(scriptArgs, "--test-only")
	}

	script := filepath.Join(dotfilesDir(), "scripts", "hg-pipeline.sh")
	cmd := exec.Command(script, scriptArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Run()

	// Try to parse JSON from last line of stdout
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	lastLine := ""
	if len(lines) > 0 {
		lastLine = lines[len(lines)-1]
	}

	var result any
	if json.Unmarshal([]byte(lastLine), &result) == nil {
		return jsonResult(result)
	}
	return jsonResult(map[string]string{
		"repo":   filepath.Base(repoPath),
		"output": stdout.String(),
		"error":  stderr.String(),
	})
}

// ---------------------------------------------------------------------------
// Tool 13: dotfiles_health_check
// ---------------------------------------------------------------------------

func healthCheck(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	script := filepath.Join(dotfilesDir(), "scripts", "hg-health.sh")
	cmd := exec.Command(script)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stdout
	cmd.Run()
	return mcp.NewToolResultText(stdout.String()), nil
}

// ---------------------------------------------------------------------------
// Tool 14: dotfiles_dep_audit
// ---------------------------------------------------------------------------

func depAudit(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	script := filepath.Join(dotfilesDir(), "scripts", "hg-dep-audit.sh")
	cmd := exec.Command(script)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stdout
	cmd.Run()
	return mcp.NewToolResultText(stdout.String()), nil
}

// ---------------------------------------------------------------------------
// Tool 15: dotfiles_workflow_sync
// ---------------------------------------------------------------------------

func workflowSync(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	dryRun, _ := args["dry_run"].(bool)
	push, _ := args["push"].(bool)

	scriptArgs := []string{}
	if dryRun {
		scriptArgs = append(scriptArgs, "--dry-run")
	}
	if push {
		scriptArgs = append(scriptArgs, "--push")
	} else {
		scriptArgs = append(scriptArgs, "--commit")
	}

	script := filepath.Join(dotfilesDir(), "scripts", "hg-workflow-sync.sh")
	cmd := exec.Command(script, scriptArgs...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stdout
	cmd.Run()
	return mcp.NewToolResultText(stdout.String()), nil
}

// ---------------------------------------------------------------------------
// Tool 16: dotfiles_go_sync
// ---------------------------------------------------------------------------

func goSync(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	dryRun, _ := args["dry_run"].(bool)
	tidy, _ := args["tidy"].(bool)

	scriptArgs := []string{}
	if dryRun {
		scriptArgs = append(scriptArgs, "--dry-run")
	}
	if tidy {
		scriptArgs = append(scriptArgs, "--tidy")
	}

	script := filepath.Join(dotfilesDir(), "scripts", "hg-go-sync.sh")
	cmd := exec.Command(script, scriptArgs...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stdout
	cmd.Run()
	return mcp.NewToolResultText(stdout.String()), nil
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

func main() {
	s := server.NewMCPServer(
		"dotfiles-mcp",
		"1.0.0",
	)

	// Tool 1: dotfiles_list_configs
	s.AddTool(
		mcp.NewTool("dotfiles_list_configs",
			mcp.WithDescription("List all dotfiles config directories with symlink health and detected format."),
		),
		listConfigs,
	)

	// Tool 2: dotfiles_validate_config
	s.AddTool(
		mcp.NewTool("dotfiles_validate_config",
			mcp.WithDescription("Validate config file content syntax (TOML or JSON)."),
			mcp.WithString("content",
				mcp.Required(),
				mcp.Description("Config file content to validate"),
			),
			mcp.WithString("format",
				mcp.Required(),
				mcp.Description("Config format"),
				mcp.Enum("toml", "json"),
			),
		),
		validateConfig,
	)

	// Tool 3: dotfiles_reload_service
	s.AddTool(
		mcp.NewTool("dotfiles_reload_service",
			mcp.WithDescription("Reload a desktop service after config changes."),
			mcp.WithString("service",
				mcp.Required(),
				mcp.Description("Service to reload"),
				mcp.Enum("hyprland", "mako", "eww", "waybar", "sway", "tmux"),
			),
		),
		reloadService,
	)

	// Tool 4: dotfiles_check_symlinks
	s.AddTool(
		mcp.NewTool("dotfiles_check_symlinks",
			mcp.WithDescription("Check health of all expected dotfiles symlinks (healthy, broken, or missing)."),
		),
		checkSymlinks,
	)

	// Tool 5: dotfiles_gh_list_personal_repos
	s.AddTool(
		mcp.NewTool("dotfiles_gh_list_personal_repos",
			mcp.WithDescription("List GitHub repos under a personal account, optionally excluding repos already in an org. Shows fork status, visibility, and language."),
			mcp.WithString("owner",
				mcp.Required(),
				mcp.Description("GitHub username whose repos to list"),
			),
			mcp.WithString("exclude_org",
				mcp.Description("Exclude repos already in this org (e.g. hairglasses-studio)"),
			),
		),
		ghListPersonalRepos,
	)

	// Tool 6: dotfiles_gh_transfer_repos
	s.AddTool(
		mcp.NewTool("dotfiles_gh_transfer_repos",
			mcp.WithDescription("Bulk transfer non-fork repos from a personal account to a GitHub org. Skips repos that already exist in the target. Set execute=true to run (dry-run by default)."),
			mcp.WithString("owner",
				mcp.Required(),
				mcp.Description("Source GitHub username"),
			),
			mcp.WithString("target_org",
				mcp.Required(),
				mcp.Description("Target GitHub organization"),
			),
			mcp.WithArray("repos",
				mcp.Required(),
				mcp.Description("Array of repo names to transfer"),
			),
			mcp.WithBoolean("execute",
				mcp.Description("Set true to execute transfers (default: dry-run)"),
			),
		),
		ghTransferRepos,
	)

	// Tool 7: dotfiles_gh_recreate_forks
	s.AddTool(
		mcp.NewTool("dotfiles_gh_recreate_forks",
			mcp.WithDescription("Recreate forked repos as fresh repos under a GitHub org. Clones each fork, squashes all history to a single initial commit on the default branch, creates a new repo in the org, pushes, and deletes the original fork. Set execute=true to run (dry-run by default)."),
			mcp.WithString("owner",
				mcp.Required(),
				mcp.Description("Source GitHub username (fork owner)"),
			),
			mcp.WithString("target_org",
				mcp.Required(),
				mcp.Description("Target GitHub organization"),
			),
			mcp.WithArray("repos",
				mcp.Required(),
				mcp.Description("Array of forked repo names to recreate"),
			),
			mcp.WithBoolean("execute",
				mcp.Description("Set true to execute (default: dry-run)"),
			),
		),
		ghRecreateForks,
	)

	// Tool 8: dotfiles_eww_restart
	s.AddTool(
		mcp.NewTool("dotfiles_eww_restart",
			mcp.WithDescription("Kill all eww and waybar processes, restart eww daemon, and open both bars (bar on DP-1, bar-secondary on DP-2). Use after editing eww config files."),
		),
		ewwRestart,
	)

	// Tool 9: dotfiles_eww_status
	s.AddTool(
		mcp.NewTool("dotfiles_eww_status",
			mcp.WithDescription("Show eww bar status: daemon health, open windows, layer surfaces, key variable values, and whether waybar is incorrectly running."),
		),
		ewwStatus,
	)

	// Tool 10: dotfiles_eww_get
	s.AddTool(
		mcp.NewTool("dotfiles_eww_get",
			mcp.WithDescription("Get the current value of an eww variable. Useful for debugging bar widgets (e.g. bar_workspaces_dp1, bar_cpu, bar_shader, rg_fleet)."),
			mcp.WithString("variable",
				mcp.Required(),
				mcp.Description("eww variable name to query"),
			),
		),
		ewwGet,
	)

	// Tool 11: dotfiles_onboard_repo
	s.AddTool(
		mcp.NewTool("dotfiles_onboard_repo",
			mcp.WithDescription("Onboard a repo with hairglasses-studio standard files (.editorconfig, CI workflows, LICENSE, CONTRIBUTING.md, pre-commit hooks). Detects language and adds appropriate config."),
			mcp.WithString("repo_path",
				mcp.Required(),
				mcp.Description("Absolute path to the repo directory"),
			),
			mcp.WithString("language",
				mcp.Description("Language override (auto-detected if omitted)"),
				mcp.Enum("auto", "go", "node", "python", "shell"),
			),
			mcp.WithBoolean("dry_run",
				mcp.Description("Preview what would be added without making changes"),
			),
		),
		onboardRepo,
	)

	// Tool 12: dotfiles_pipeline_run
	s.AddTool(
		mcp.NewTool("dotfiles_pipeline_run",
			mcp.WithDescription("Run the build+test pipeline (hg-pipeline.sh) on a repo. Supports Go, Node.js, and Python. Returns JSON results with per-step timing."),
			mcp.WithString("repo_path",
				mcp.Required(),
				mcp.Description("Absolute path to the repo directory"),
			),
			mcp.WithBoolean("build_only",
				mcp.Description("Only run the build step"),
			),
			mcp.WithBoolean("test_only",
				mcp.Description("Only run the test step"),
			),
		),
		pipelineRun,
	)

	// Tool 13: dotfiles_health_check
	s.AddTool(
		mcp.NewTool("dotfiles_health_check",
			mcp.WithDescription("Run org-wide health dashboard across all hairglasses-studio repos. Shows build status, Go version, pipeline.mk inclusion, and CI workflow presence for every repo."),
		),
		healthCheck,
	)

	// Tool 14: dotfiles_dep_audit
	s.AddTool(
		mcp.NewTool("dotfiles_dep_audit",
			mcp.WithDescription("Audit Go dependency version skew across all hairglasses-studio repos. Reports which deps are unified vs which have version drift."),
		),
		depAudit,
	)

	// Tool 15: dotfiles_workflow_sync
	s.AddTool(
		mcp.NewTool("dotfiles_workflow_sync",
			mcp.WithDescription("Sync CI workflow files across all repos from canonical sources. Detects stale workflows and optionally updates, commits, and pushes."),
			mcp.WithBoolean("dry_run",
				mcp.Description("Preview changes without modifying files"),
			),
			mcp.WithBoolean("push",
				mcp.Description("Commit and push changes to each repo"),
			),
		),
		workflowSync,
	)

	// Tool 16: dotfiles_go_sync
	s.AddTool(
		mcp.NewTool("dotfiles_go_sync",
			mcp.WithDescription("Sync Go version across all repos to match dotfiles/make/go-version. Updates go.mod and optionally runs go mod tidy."),
			mcp.WithBoolean("dry_run",
				mcp.Description("Preview which repos would be updated"),
			),
			mcp.WithBoolean("tidy",
				mcp.Description("Run go mod tidy after updating go.mod"),
			),
		),
		goSync,
	)

	if err := server.ServeStdio(s); err != nil {
		log.Fatal(err)
	}
}
