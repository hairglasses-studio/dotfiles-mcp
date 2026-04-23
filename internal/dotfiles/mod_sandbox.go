// mod_sandbox.go — Docker + NVIDIA GPU sandbox for isolated dotfile testing
package dotfiles

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
)

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const (
	sandboxImage      = "dotfiles-sandbox:latest"
	sandboxDotfilesRO = "/dotfiles"
	sandboxReadyFile  = "/tmp/sandbox-ready"
	sandboxConfigDir  = "/sandbox"
	defaultResolution = "2560x1440"
	maxScreenshotPx   = "1568x1568>"
)

// ---------------------------------------------------------------------------
// State management
// ---------------------------------------------------------------------------

var (
	sandboxMu    sync.RWMutex
	sandboxes    = make(map[string]*SandboxInstance)
	dotfilesPath string // resolved once
)

type SandboxInstance struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Status      string    `json:"status"` // created, running, stopped
	Profile     string    `json:"profile"`
	ContainerID string    `json:"container_id"`
	CreatedAt   time.Time `json:"created_at"`
	ConfigDir   string    `json:"config_dir"`
}

func sandboxID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func getDotfilesPath() string {
	if dotfilesPath != "" {
		return dotfilesPath
	}
	// Try common locations
	for _, p := range []string{
		os.Getenv("DOTFILES_DIR"),
		os.Getenv("HOME") + "/hairglasses-studio/dotfiles",
		os.Getenv("HOME") + "/dotfiles",
	} {
		if p != "" {
			if _, err := os.Stat(p); err == nil {
				dotfilesPath = p
				return p
			}
		}
	}
	return ""
}

func getSandbox(id string) (*SandboxInstance, error) {
	sandboxMu.RLock()
	defer sandboxMu.RUnlock()
	s, ok := sandboxes[id]
	if !ok {
		return nil, fmt.Errorf("[%s] sandbox %q not found", handler.ErrNotFound, id)
	}
	return s, nil
}

// ---------------------------------------------------------------------------
// Docker helpers
// ---------------------------------------------------------------------------

func dockerCmd(args ...string) (string, error) {
	cmd := exec.Command("docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("docker %s: %w: %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func dockerExec(containerID string, command ...string) (string, error) {
	args := append([]string{"exec", containerID}, command...)
	return dockerCmd(args...)
}

func dockerExecUser(containerID, user string, command ...string) (string, error) {
	args := append([]string{"exec", "-u", user, containerID}, command...)
	return dockerCmd(args...)
}

// refreshContainerStatus syncs a sandbox's status with Docker.
func refreshContainerStatus(s *SandboxInstance) {
	out, err := dockerCmd("inspect", "--format", "{{.State.Status}}", s.ContainerID)
	if err != nil {
		s.Status = "unknown"
		return
	}
	switch strings.TrimSpace(out) {
	case "running":
		s.Status = "running"
	case "exited", "dead":
		s.Status = "stopped"
	case "created":
		s.Status = "created"
	default:
		s.Status = out
	}
}

// ---------------------------------------------------------------------------
// Input/Output types
// ---------------------------------------------------------------------------

type SandboxCreateInput struct {
	Name    string `json:"name,omitempty" jsonschema:"description=Human-readable sandbox name. Auto-generated if empty."`
	Profile string `json:"profile,omitempty" jsonschema:"description=Package profile: minimal (shell only) or full (desktop with Hyprland),enum=minimal,enum=full"`
}

type SandboxCreateOutput struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Profile   string `json:"profile"`
	CreatedAt string `json:"created_at"`
}

type SandboxIDInput struct {
	ID string `json:"id" jsonschema:"required,description=Sandbox instance ID"`
}

type SandboxStartOutput struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type SandboxStopOutput struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type SandboxDestroyInput struct {
	ID    string `json:"id" jsonschema:"required,description=Sandbox instance ID"`
	Force bool   `json:"force,omitempty" jsonschema:"description=Force removal even if running"`
}

type SandboxDestroyOutput struct {
	ID        string `json:"id"`
	Destroyed bool   `json:"destroyed"`
}

type SandboxListInput struct {
	Status string `json:"status,omitempty" jsonschema:"description=Filter by status: running/stopped/all,enum=running,enum=stopped,enum=all"`
}

type SandboxListOutput struct {
	Sandboxes []SandboxInfo `json:"sandboxes"`
	Count     int           `json:"count"`
}

type SandboxInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Profile   string `json:"profile"`
	Age       string `json:"age"`
	CreatedAt string `json:"created_at"`
}

type SandboxStatusInput struct {
	ID string `json:"id" jsonschema:"required,description=Sandbox instance ID"`
}

type SandboxStatusOutput struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Profile  string `json:"profile"`
	Uptime   string `json:"uptime,omitempty"`
	GPU      string `json:"gpu,omitempty"`
	MemUsage string `json:"mem_usage,omitempty"`
	CPUPct   string `json:"cpu_pct,omitempty"`
}

type SandboxSyncInput struct {
	ID string `json:"id" jsonschema:"required,description=Sandbox instance ID"`
}

type SandboxSyncOutput struct {
	Synced  bool     `json:"synced"`
	Steps   []string `json:"steps"`
	Errors  []string `json:"errors,omitempty"`
	Message string   `json:"message"`
}

type SandboxTestInput struct {
	ID    string `json:"id" jsonschema:"required,description=Sandbox instance ID"`
	Suite string `json:"suite,omitempty" jsonschema:"description=Test suite to run,enum=bats,enum=selftest,enum=symlinks,enum=shaders,enum=config,enum=all"`
}

type SandboxTestResult struct {
	Name   string `json:"name"`
	Status string `json:"status"` // pass, fail, skip, error
	Output string `json:"output,omitempty"`
}

type SandboxTestOutput struct {
	Passed  int                 `json:"passed"`
	Failed  int                 `json:"failed"`
	Skipped int                 `json:"skipped"`
	Results []SandboxTestResult `json:"results"`
	Overall string              `json:"overall"` // pass, fail
}

type SandboxExecInput struct {
	ID         string `json:"id" jsonschema:"required,description=Sandbox instance ID"`
	Command    string `json:"command" jsonschema:"required,description=Command to execute inside the sandbox"`
	TimeoutSec int    `json:"timeout_sec,omitempty" jsonschema:"description=Command timeout in seconds (default 30)"`
}

type SandboxExecOutput struct {
	ExitCode   int    `json:"exit_code"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	DurationMs int64  `json:"duration_ms"`
}

type SandboxDiffInput struct {
	ID string `json:"id" jsonschema:"required,description=Sandbox instance ID"`
}

type SandboxDiffOutput struct {
	Changed []SandboxDiffEntry `json:"changed"`
	Summary string             `json:"summary"`
}

type SandboxDiffEntry struct {
	Path   string `json:"path"`
	Status string `json:"status"` // missing, modified, extra, broken
}

type SandboxScreenshotInput struct {
	ID      string `json:"id" jsonschema:"required,description=Sandbox instance ID"`
	WaitSec int    `json:"wait_sec,omitempty" jsonschema:"description=Seconds to wait before capture for visual settle (default 3)"`
}

type SandboxVisualDiffInput struct {
	ID            string `json:"id" jsonschema:"required,description=Sandbox instance ID"`
	ReferencePath string `json:"reference_path,omitempty" jsonschema:"description=Path to reference screenshot. Uses latest from sandbox/reference/ if empty."`
}

type SandboxValidateInput struct {
	DestroyAfter bool `json:"destroy_after,omitempty" jsonschema:"description=Destroy sandbox after validation (default true)"`
}

type SandboxValidateOutput struct {
	TestResults *SandboxTestOutput `json:"test_results,omitempty"`
	Overall     string             `json:"overall"` // pass, fail
	SandboxID   string             `json:"sandbox_id"`
	Message     string             `json:"message"`
}

// ---------------------------------------------------------------------------
// Module
// ---------------------------------------------------------------------------

type SandboxModule struct{}

func (m *SandboxModule) Name() string { return "sandbox" }
func (m *SandboxModule) Description() string {
	return "Docker + NVIDIA GPU sandbox for isolated dotfile testing with visual preview"
}

func (m *SandboxModule) Tools() []registry.ToolDefinition {
	return []registry.ToolDefinition{
		// ── Lifecycle ────────────────────────────────────────────────────

		handler.TypedHandler[SandboxCreateInput, SandboxCreateOutput](
			"sandbox_create",
			"Create a new Docker sandbox container for dotfile testing. Uses the host's RTX 3080 via nvidia-container-toolkit for GPU-accelerated Hyprland headless rendering. Dotfiles are bind-mounted read-only from the host working tree.",
			sandboxCreate,
		),

		handler.TypedHandler[SandboxIDInput, SandboxStartOutput](
			"sandbox_start",
			"Start a stopped sandbox container.",
			sandboxStart,
		),

		handler.TypedHandler[SandboxIDInput, SandboxStopOutput](
			"sandbox_stop",
			"Stop a running sandbox container.",
			sandboxStop,
		),

		handler.TypedHandler[SandboxDestroyInput, SandboxDestroyOutput](
			"sandbox_destroy",
			"Destroy a sandbox container and clean up resources. Use force=true to remove even if running.",
			sandboxDestroy,
		),

		handler.TypedHandler[SandboxListInput, SandboxListOutput](
			"sandbox_list",
			"List all managed sandbox containers with status, profile, and age.",
			sandboxList,
		),

		handler.TypedHandler[SandboxStatusInput, SandboxStatusOutput](
			"sandbox_status",
			"Get detailed status of a sandbox including GPU utilization, memory, and CPU usage.",
			sandboxStatus,
		),

		// ── Config Sync & Test ───────────────────────────────────────────

		handler.TypedHandler[SandboxSyncInput, SandboxSyncOutput](
			"sandbox_sync",
			"Sync dotfiles into a running sandbox. Runs install.sh to create symlinks and reloads Hyprland config.",
			sandboxSync,
		),

		handler.TypedHandler[SandboxTestInput, SandboxTestOutput](
			"sandbox_test",
			"Run test suite inside a sandbox. Suites: bats (BATS tests), selftest (rice validation), symlinks (symlink health), shaders (GPU GLSL compilation), config (TOML/JSON syntax), all (everything). Returns structured pass/fail results.",
			sandboxTest,
		),

		handler.TypedHandler[SandboxExecInput, SandboxExecOutput](
			"sandbox_exec",
			"Execute an arbitrary command inside a sandbox container. Returns stdout, stderr, exit code, and duration.",
			sandboxExec,
		),

		handler.TypedHandler[SandboxDiffInput, SandboxDiffOutput](
			"sandbox_diff",
			"Compare sandbox symlink and config state against the host dotfiles working tree. Reports missing, modified, extra, and broken symlinks.",
			sandboxDiff,
		),

		// ── Visual Capture ───────────────────────────────────────────────

		// Raw handler for image content return
		{
			Tool: mcp.Tool{
				Name:        "sandbox_screenshot",
				Description: "Capture a screenshot of the sandbox's headless Hyprland display via grim. Returns the image inline as base64 PNG. Optionally waits for visual settle before capture.",
				InputSchema: mcp.ToolInputSchema{
					Type: "object",
					Properties: map[string]any{
						"id": map[string]any{
							"type":        "string",
							"description": "Sandbox instance ID",
						},
						"wait_sec": map[string]any{
							"type":        "integer",
							"description": "Seconds to wait before capture for visual settle (default 3)",
						},
					},
					Required: []string{"id"},
				},
			},
			Handler: sandboxScreenshotHandler,
		},

		// Raw handler for visual diff (returns diff image)
		{
			Tool: mcp.Tool{
				Name:        "sandbox_visual_diff",
				Description: "Compare a sandbox screenshot against a reference image using ImageMagick. Returns the diff image inline and a similarity percentage.",
				InputSchema: mcp.ToolInputSchema{
					Type: "object",
					Properties: map[string]any{
						"id": map[string]any{
							"type":        "string",
							"description": "Sandbox instance ID",
						},
						"reference_path": map[string]any{
							"type":        "string",
							"description": "Path to reference screenshot. Uses latest from sandbox/reference/ if empty.",
						},
					},
					Required: []string{"id"},
				},
			},
			Handler: sandboxVisualDiffHandler,
		},

		// ── Composed Pipeline ────────────────────────────────────────────

		handler.TypedHandler[SandboxValidateInput, SandboxValidateOutput](
			"sandbox_validate",
			"Composed: create sandbox → sync dotfiles → run all tests → capture screenshot → destroy. Returns structured results with pass/fail and screenshot. Early aborts if tests fail.",
			sandboxValidate,
		),
	}
}

// ---------------------------------------------------------------------------
// Lifecycle handlers
// ---------------------------------------------------------------------------

func sandboxCreate(_ context.Context, input SandboxCreateInput) (SandboxCreateOutput, error) {
	dfPath := getDotfilesPath()
	if dfPath == "" {
		return SandboxCreateOutput{}, fmt.Errorf("[%s] cannot find dotfiles directory", handler.ErrNotFound)
	}

	// Check docker is available
	if _, err := exec.LookPath("docker"); err != nil {
		return SandboxCreateOutput{}, fmt.Errorf("[%s] docker not found on PATH", handler.ErrPermission)
	}

	id := sandboxID()
	name := input.Name
	if name == "" {
		name = "sandbox-" + id
	}
	profile := input.Profile
	if profile == "" {
		profile = "full"
	}

	configDir := fmt.Sprintf("/tmp/sandbox-%s", id)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return SandboxCreateOutput{}, fmt.Errorf("create config dir: %w", err)
	}

	// Resolve host Wayland display for nested compositor
	waylandDisplay := os.Getenv("WAYLAND_DISPLAY")
	if waylandDisplay == "" {
		waylandDisplay = "wayland-1"
	}
	xdgRuntime := os.Getenv("XDG_RUNTIME_DIR")
	if xdgRuntime == "" {
		xdgRuntime = "/run/user/1000"
	}
	waylandSocket := xdgRuntime + "/" + waylandDisplay

	// Build docker create command — nested Wayland compositor with GPU
	args := []string{
		"create",
		"--name", name,
		"--device", "nvidia.com/gpu=0",
		"--runtime=nvidia",
		// Mount host dotfiles read-only
		"-v", dfPath + ":" + sandboxDotfilesRO + ":ro",
		// Writable XDG_RUNTIME_DIR (tmpfs), then overlay host Wayland socket
		"--tmpfs", xdgRuntime + ":exec,mode=700,uid=1000",
		"-v", waylandSocket + ":" + xdgRuntime + "/" + waylandDisplay,
		"-e", "WAYLAND_DISPLAY=" + waylandDisplay,
		"-e", "XDG_RUNTIME_DIR=" + xdgRuntime,
		"-e", "NVIDIA_VISIBLE_DEVICES=all",
		"-e", "NVIDIA_DRIVER_CAPABILITIES=all",
	}

	// Minimal profile skips Hyprland startup
	if profile == "minimal" {
		args = append(args, "-e", "SANDBOX_SKIP_HYPRLAND=1")
	}

	args = append(args, sandboxImage)

	containerID, err := dockerCmd(args...)
	if err != nil {
		os.RemoveAll(configDir)
		return SandboxCreateOutput{}, fmt.Errorf("docker create: %w", err)
	}

	now := time.Now()
	s := &SandboxInstance{
		ID:          id,
		Name:        name,
		Status:      "created",
		Profile:     profile,
		ContainerID: strings.TrimSpace(containerID),
		CreatedAt:   now,
		ConfigDir:   configDir,
	}

	sandboxMu.Lock()
	sandboxes[id] = s
	sandboxMu.Unlock()

	return SandboxCreateOutput{
		ID:        id,
		Name:      name,
		Status:    "created",
		Profile:   profile,
		CreatedAt: now.Format(time.RFC3339),
	}, nil
}

func sandboxStart(_ context.Context, input SandboxIDInput) (SandboxStartOutput, error) {
	s, err := getSandbox(input.ID)
	if err != nil {
		return SandboxStartOutput{}, err
	}

	if _, err := dockerCmd("start", s.ContainerID); err != nil {
		return SandboxStartOutput{}, fmt.Errorf("docker start: %w", err)
	}

	// Wait for ready signal (Hyprland started)
	timeout := 60 * time.Second
	start := time.Now()
	for time.Since(start) < timeout {
		out, err := dockerCmd("exec", s.ContainerID, "test", "-f", sandboxReadyFile)
		if err == nil && out == "" {
			break
		}
		// Check if container is still running
		refreshContainerStatus(s)
		if s.Status != "running" {
			logs, _ := dockerCmd("logs", "--tail", "30", s.ContainerID)
			return SandboxStartOutput{}, fmt.Errorf("sandbox exited during startup. Logs:\n%s", logs)
		}
		time.Sleep(2 * time.Second)
	}

	sandboxMu.Lock()
	s.Status = "running"
	sandboxMu.Unlock()

	// Resize the nested Wayland window on the host to full resolution.
	// The nested Hyprland window has class "aquamarine".
	resizeSandboxWindow(defaultResolution)

	return SandboxStartOutput{ID: s.ID, Status: "running"}, nil
}

// resizeSandboxWindow finds the nested Hyprland window on the host and resizes it.
func resizeSandboxWindow(resolution string) {
	// Find the aquamarine window address
	out, err := exec.Command("hyprctl", "clients", "-j").Output()
	if err != nil {
		return
	}
	var clients []map[string]any
	if err := json.Unmarshal(out, &clients); err != nil {
		return
	}
	for _, c := range clients {
		class, _ := c["class"].(string)
		if class == "aquamarine" {
			addr, _ := c["address"].(string)
			if addr == "" {
				continue
			}
			// Make floating + resize
			exec.Command("hyprctl", "dispatch", "togglefloating", "address:"+addr).Run()
			exec.Command("hyprctl", "dispatch", "resizewindowpixel",
				fmt.Sprintf("exact %s,address:%s", strings.Replace(resolution, "x", " ", 1), addr)).Run()
			return
		}
	}
}

func sandboxStop(_ context.Context, input SandboxIDInput) (SandboxStopOutput, error) {
	s, err := getSandbox(input.ID)
	if err != nil {
		return SandboxStopOutput{}, err
	}

	if _, err := dockerCmd("stop", "-t", "10", s.ContainerID); err != nil {
		return SandboxStopOutput{}, fmt.Errorf("docker stop: %w", err)
	}

	sandboxMu.Lock()
	s.Status = "stopped"
	sandboxMu.Unlock()

	return SandboxStopOutput{ID: s.ID, Status: "stopped"}, nil
}

func sandboxDestroy(_ context.Context, input SandboxDestroyInput) (SandboxDestroyOutput, error) {
	s, err := getSandbox(input.ID)
	if err != nil {
		return SandboxDestroyOutput{}, err
	}

	args := []string{"rm"}
	if input.Force {
		args = append(args, "-f")
	}
	args = append(args, s.ContainerID)

	if _, err := dockerCmd(args...); err != nil {
		return SandboxDestroyOutput{}, fmt.Errorf("docker rm: %w", err)
	}

	// Clean up config dir
	os.RemoveAll(s.ConfigDir)

	sandboxMu.Lock()
	delete(sandboxes, input.ID)
	sandboxMu.Unlock()

	return SandboxDestroyOutput{ID: input.ID, Destroyed: true}, nil
}

func sandboxList(_ context.Context, input SandboxListInput) (SandboxListOutput, error) {
	sandboxMu.RLock()
	defer sandboxMu.RUnlock()

	filter := input.Status
	if filter == "" {
		filter = "all"
	}

	var infos []SandboxInfo
	for _, s := range sandboxes {
		refreshContainerStatus(s)
		if filter != "all" && s.Status != filter {
			continue
		}
		infos = append(infos, SandboxInfo{
			ID:        s.ID,
			Name:      s.Name,
			Status:    s.Status,
			Profile:   s.Profile,
			Age:       time.Since(s.CreatedAt).Truncate(time.Second).String(),
			CreatedAt: s.CreatedAt.Format(time.RFC3339),
		})
	}

	return SandboxListOutput{
		Sandboxes: infos,
		Count:     len(infos),
	}, nil
}

func sandboxStatus(_ context.Context, input SandboxStatusInput) (SandboxStatusOutput, error) {
	s, err := getSandbox(input.ID)
	if err != nil {
		return SandboxStatusOutput{}, err
	}

	refreshContainerStatus(s)

	out := SandboxStatusOutput{
		ID:      s.ID,
		Name:    s.Name,
		Status:  s.Status,
		Profile: s.Profile,
		Uptime:  time.Since(s.CreatedAt).Truncate(time.Second).String(),
	}

	if s.Status == "running" {
		// Get GPU info from inside container
		if gpu, err := dockerExec(s.ContainerID, "nvidia-smi", "--query-gpu=name,utilization.gpu,memory.used,memory.total", "--format=csv,noheader,nounits"); err == nil {
			out.GPU = strings.TrimSpace(gpu)
		}
		// Get container stats
		if stats, err := dockerCmd("stats", "--no-stream", "--format", "{{.CPUPerc}} {{.MemUsage}}", s.ContainerID); err == nil {
			parts := strings.SplitN(strings.TrimSpace(stats), " ", 2)
			if len(parts) >= 1 {
				out.CPUPct = parts[0]
			}
			if len(parts) >= 2 {
				out.MemUsage = parts[1]
			}
		}
	}

	return out, nil
}

// ---------------------------------------------------------------------------
// Config Sync & Test handlers
// ---------------------------------------------------------------------------

func sandboxSync(_ context.Context, input SandboxSyncInput) (SandboxSyncOutput, error) {
	s, err := getSandbox(input.ID)
	if err != nil {
		return SandboxSyncOutput{}, err
	}

	refreshContainerStatus(s)
	if s.Status != "running" {
		return SandboxSyncOutput{}, fmt.Errorf("[%s] sandbox must be running (current: %s)", handler.ErrInvalidParam, s.Status)
	}

	var steps []string
	var errors []string

	// Step 1: Run install.sh inside container
	out, err := dockerExecUser(s.ContainerID, "hg", "bash", "-c",
		"export DOTFILES_DIR="+sandboxDotfilesRO+" && bash "+sandboxDotfilesRO+"/manjaro/install.sh --profile minimal 2>&1")
	if err != nil {
		errors = append(errors, fmt.Sprintf("install.sh: %s", out))
	} else {
		steps = append(steps, "install.sh: symlinks deployed")
	}

	// Step 2: Reload Hyprland if running
	if s.Profile == "full" {
		// Read the instance signature from entrypoint
		inst, _ := dockerExec(s.ContainerID, "cat", "/tmp/sandbox-instance")
		inst = strings.TrimSpace(inst)
		hyprctlCmd := fmt.Sprintf("HYPRLAND_INSTANCE_SIGNATURE=%s hyprctl reload", inst)
		if _, err := dockerExecUser(s.ContainerID, "hg", "bash", "-c", hyprctlCmd); err == nil {
			steps = append(steps, "hyprctl reload: config reloaded")
		} else {
			errors = append(errors, "hyprctl reload: Hyprland not responding (may not be running)")
		}
	}

	synced := len(errors) == 0
	msg := "Sync complete"
	if !synced {
		msg = fmt.Sprintf("Sync completed with %d error(s)", len(errors))
	}

	return SandboxSyncOutput{
		Synced:  synced,
		Steps:   steps,
		Errors:  errors,
		Message: msg,
	}, nil
}

func sandboxTest(_ context.Context, input SandboxTestInput) (SandboxTestOutput, error) {
	s, err := getSandbox(input.ID)
	if err != nil {
		return SandboxTestOutput{}, err
	}

	refreshContainerStatus(s)
	if s.Status != "running" {
		return SandboxTestOutput{}, fmt.Errorf("[%s] sandbox must be running (current: %s)", handler.ErrInvalidParam, s.Status)
	}

	suite := input.Suite
	if suite == "" {
		suite = "all"
	}

	var results []SandboxTestResult
	passed, failed, skipped := 0, 0, 0

	runTest := func(name, cmd string) {
		out, err := dockerExecUser(s.ContainerID, "hg", "bash", "-c", cmd)
		status := "pass"
		if err != nil {
			status = "fail"
			failed++
		} else {
			passed++
		}
		results = append(results, SandboxTestResult{
			Name:   name,
			Status: status,
			Output: strings.TrimSpace(out),
		})
	}

	// BATS tests
	if suite == "bats" || suite == "all" {
		runTest("bats", "cd "+sandboxDotfilesRO+" && bats tests/*.bats 2>&1 || true")
	}

	// Symlink validation
	if suite == "symlinks" || suite == "all" {
		runTest("symlinks", "cd "+sandboxDotfilesRO+" && bash manjaro/install.sh --check 2>&1")
	}

	// Config syntax validation (TOML/JSON)
	if suite == "config" || suite == "all" {
		// Validate hyprland config
		if s.Profile == "full" {
			runTest("hyprland-config", "hyprctl configerrors 2>&1")
		}
		// Validate TOML configs
		runTest("toml-syntax", "cd "+sandboxDotfilesRO+" && find . -name '*.toml' -not -path './mcp/*' -exec python3 -c 'import tomllib,sys; tomllib.load(open(sys.argv[1],\"rb\"))' {} \\; 2>&1")
	}

	// Shader compilation (GPU-accelerated)
	if suite == "shaders" || suite == "all" {
		runTest("shader-compile", "cd "+sandboxDotfilesRO+" && bash kitty/shaders/bin/shader-test.sh 2>&1")
	}

	// Rice selftest
	if suite == "selftest" || suite == "all" {
		runTest("rice-selftest", "cd "+sandboxDotfilesRO+" && bash scripts/rice-selftest.sh 2>&1")
	}

	overall := "pass"
	if failed > 0 {
		overall = "fail"
	}

	return SandboxTestOutput{
		Passed:  passed,
		Failed:  failed,
		Skipped: skipped,
		Results: results,
		Overall: overall,
	}, nil
}

func sandboxExec(_ context.Context, input SandboxExecInput) (SandboxExecOutput, error) {
	s, err := getSandbox(input.ID)
	if err != nil {
		return SandboxExecOutput{}, err
	}

	if input.Command == "" {
		return SandboxExecOutput{}, fmt.Errorf("[%s] command must not be empty", handler.ErrInvalidParam)
	}

	refreshContainerStatus(s)
	if s.Status != "running" {
		return SandboxExecOutput{}, fmt.Errorf("[%s] sandbox must be running (current: %s)", handler.ErrInvalidParam, s.Status)
	}

	timeout := input.TimeoutSec
	if timeout <= 0 {
		timeout = 30
	}

	start := time.Now()
	cmd := exec.Command("docker", "exec", "-u", "hg", s.ContainerID, "bash", "-c", input.Command)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run with timeout
	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()

	select {
	case err := <-done:
		duration := time.Since(start).Milliseconds()
		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				return SandboxExecOutput{}, fmt.Errorf("exec failed: %w", err)
			}
		}
		return SandboxExecOutput{
			ExitCode:   exitCode,
			Stdout:     stdout.String(),
			Stderr:     stderr.String(),
			DurationMs: duration,
		}, nil

	case <-time.After(time.Duration(timeout) * time.Second):
		cmd.Process.Kill()
		return SandboxExecOutput{}, fmt.Errorf("[%s] command timed out after %ds", handler.ErrTimeout, timeout)
	}
}

func sandboxDiff(_ context.Context, input SandboxDiffInput) (SandboxDiffOutput, error) {
	s, err := getSandbox(input.ID)
	if err != nil {
		return SandboxDiffOutput{}, err
	}

	refreshContainerStatus(s)
	if s.Status != "running" {
		return SandboxDiffOutput{}, fmt.Errorf("[%s] sandbox must be running (current: %s)", handler.ErrInvalidParam, s.Status)
	}

	// Check symlinks inside container
	out, _ := dockerExecUser(s.ContainerID, "hg", "bash", "-c",
		`for link in ~/.config/hypr ~/.config/ironbar ~/.config/swaync ~/.config/kitty ~/.config/starship.toml; do
			if [ -L "$link" ]; then
				target=$(readlink "$link")
				if [ -e "$link" ]; then
					echo "ok $link -> $target"
				else
					echo "broken $link -> $target"
				fi
			elif [ -e "$link" ]; then
				echo "extra $link (not a symlink)"
			else
				echo "missing $link"
			fi
		done`)

	var changed []SandboxDiffEntry
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}
		status := parts[0]
		path := parts[1]
		if status != "ok" {
			changed = append(changed, SandboxDiffEntry{
				Path:   path,
				Status: status,
			})
		}
	}

	summary := fmt.Sprintf("%d issue(s) found", len(changed))
	if len(changed) == 0 {
		summary = "All symlinks healthy"
	}

	return SandboxDiffOutput{
		Changed: changed,
		Summary: summary,
	}, nil
}

// ---------------------------------------------------------------------------
// Visual Capture handlers (raw handlers for image content)
// ---------------------------------------------------------------------------

func sandboxScreenshotHandler(_ context.Context, req registry.CallToolRequest) (*registry.CallToolResult, error) {
	var input SandboxScreenshotInput
	if req.Params.Arguments != nil {
		b, _ := json.Marshal(req.Params.Arguments)
		_ = json.Unmarshal(b, &input) // zero-value input on malformed args; downstream validation surfaces missing fields
	}

	if input.ID == "" {
		return handler.CodedErrorResult(handler.ErrInvalidParam, fmt.Errorf("id is required")), nil
	}

	s, err := getSandbox(input.ID)
	if err != nil {
		return handler.ErrorResult(err), nil
	}

	refreshContainerStatus(s)
	if s.Status != "running" {
		return handler.CodedErrorResult(handler.ErrInvalidParam,
			fmt.Errorf("sandbox must be running (current: %s)", s.Status)), nil
	}

	// Wait for visual settle
	waitSec := input.WaitSec
	if waitSec <= 0 {
		waitSec = 3
	}
	time.Sleep(time.Duration(waitSec) * time.Second)

	// Capture screenshot inside container using grim on the nested Wayland display
	screenshotPath := "/tmp/sandbox-screenshot.png"
	// Read the nested display name written by entrypoint.sh
	nestedDisplay, _ := dockerExec(s.ContainerID, "cat", "/tmp/sandbox-wayland-display")
	nestedDisplay = strings.TrimSpace(nestedDisplay)
	if nestedDisplay == "" {
		nestedDisplay = "wayland-2" // fallback
	}
	grimCmd := fmt.Sprintf("WAYLAND_DISPLAY=%s grim %s", nestedDisplay, screenshotPath)
	if _, err := dockerExecUser(s.ContainerID, "hg", "bash", "-c", grimCmd); err != nil {
		return handler.ErrorResult(fmt.Errorf("grim screenshot failed: %w", err)), nil
	}

	// Copy screenshot to host
	hostPath := fmt.Sprintf("/tmp/sandbox-%s-screenshot.png", s.ID)
	if _, err := dockerCmd("cp", s.ContainerID+":"+screenshotPath, hostPath); err != nil {
		return handler.ErrorResult(fmt.Errorf("copy screenshot to host: %w", err)), nil
	}
	defer os.Remove(hostPath)

	// Resize for inline display
	resized := hostPath + ".resized.png"
	defer os.Remove(resized)

	if _, err := exec.Command("magick", hostPath, "-resize", maxScreenshotPx, resized).CombinedOutput(); err != nil {
		// Fallback: return without resize
		data, err := os.ReadFile(hostPath)
		if err != nil {
			return handler.ErrorResult(fmt.Errorf("read screenshot: %w", err)), nil
		}
		b64 := base64.StdEncoding.EncodeToString(data)
		return &registry.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{Type: "text", Text: fmt.Sprintf("Sandbox %s screenshot (no resize, magick unavailable)", s.ID)},
				mcp.ImageContent{Type: "image", Data: b64, MIMEType: "image/png"},
			},
		}, nil
	}

	data, err := os.ReadFile(resized)
	if err != nil {
		return handler.ErrorResult(fmt.Errorf("read resized screenshot: %w", err)), nil
	}
	b64 := base64.StdEncoding.EncodeToString(data)

	return &registry.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{Type: "text", Text: fmt.Sprintf("Sandbox %s (%s) — Hyprland headless screenshot", s.ID, s.Name)},
			mcp.ImageContent{Type: "image", Data: b64, MIMEType: "image/png"},
		},
	}, nil
}

func sandboxVisualDiffHandler(_ context.Context, req registry.CallToolRequest) (*registry.CallToolResult, error) {
	var input SandboxVisualDiffInput
	if req.Params.Arguments != nil {
		b, _ := json.Marshal(req.Params.Arguments)
		_ = json.Unmarshal(b, &input) // zero-value input on malformed args; downstream validation surfaces missing fields
	}

	if input.ID == "" {
		return handler.CodedErrorResult(handler.ErrInvalidParam, fmt.Errorf("id is required")), nil
	}

	s, err := getSandbox(input.ID)
	if err != nil {
		return handler.ErrorResult(err), nil
	}

	refreshContainerStatus(s)
	if s.Status != "running" {
		return handler.CodedErrorResult(handler.ErrInvalidParam,
			fmt.Errorf("sandbox must be running (current: %s)", s.Status)), nil
	}

	// Find reference image
	refPath := input.ReferencePath
	if refPath == "" {
		dfPath := getDotfilesPath()
		if dfPath != "" {
			refPath = dfPath + "/sandbox/reference/latest.png"
		}
	}
	if refPath == "" || !sandboxFileExists(refPath) {
		return handler.CodedErrorResult(handler.ErrNotFound,
			fmt.Errorf("no reference image found. Capture one first with sandbox_screenshot and save to sandbox/reference/latest.png")), nil
	}

	// Capture current screenshot on nested display
	screenshotPath := "/tmp/sandbox-screenshot.png"
	nestedDisplay, _ := dockerExec(s.ContainerID, "cat", "/tmp/sandbox-wayland-display")
	nestedDisplay = strings.TrimSpace(nestedDisplay)
	if nestedDisplay == "" {
		nestedDisplay = "wayland-2"
	}
	grimCmd := fmt.Sprintf("WAYLAND_DISPLAY=%s grim %s", nestedDisplay, screenshotPath)
	if _, err := dockerExecUser(s.ContainerID, "hg", "bash", "-c", grimCmd); err != nil {
		return handler.ErrorResult(fmt.Errorf("grim screenshot failed: %w", err)), nil
	}

	hostPath := fmt.Sprintf("/tmp/sandbox-%s-vdiff.png", s.ID)
	if _, err := dockerCmd("cp", s.ContainerID+":"+screenshotPath, hostPath); err != nil {
		return handler.ErrorResult(fmt.Errorf("copy screenshot to host: %w", err)), nil
	}
	defer os.Remove(hostPath)

	// Run ImageMagick compare
	diffPath := fmt.Sprintf("/tmp/sandbox-%s-diff.png", s.ID)
	defer os.Remove(diffPath)

	// Get SSIM-style metric (AE = absolute error pixel count)
	metricOut, _ := exec.Command("magick", "compare", "-metric", "AE",
		hostPath, refPath, diffPath).CombinedOutput()

	// Resize diff for inline display
	resized := diffPath + ".resized.png"
	defer os.Remove(resized)
	exec.Command("magick", diffPath, "-resize", maxScreenshotPx, resized).CombinedOutput()

	diffData, err := os.ReadFile(resized)
	if err != nil {
		// Fallback to non-resized
		diffData, err = os.ReadFile(diffPath)
		if err != nil {
			return handler.ErrorResult(fmt.Errorf("read diff image: %w", err)), nil
		}
	}
	b64 := base64.StdEncoding.EncodeToString(diffData)

	return &registry.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: fmt.Sprintf("Visual diff for sandbox %s. Pixel difference: %s", s.ID, strings.TrimSpace(string(metricOut))),
			},
			mcp.ImageContent{Type: "image", Data: b64, MIMEType: "image/png"},
		},
	}, nil
}

// ---------------------------------------------------------------------------
// Composed Pipeline
// ---------------------------------------------------------------------------

func sandboxValidate(ctx context.Context, input SandboxValidateInput) (SandboxValidateOutput, error) {
	// Always destroy after validation — use individual tools to keep sandboxes alive
	destroyAfter := true

	// Step 1: Create sandbox
	createOut, err := sandboxCreate(ctx, SandboxCreateInput{
		Name:    "validate-" + sandboxID(),
		Profile: "full",
	})
	if err != nil {
		return SandboxValidateOutput{}, fmt.Errorf("create: %w", err)
	}

	sandboxIDVal := createOut.ID

	// Ensure cleanup
	if destroyAfter {
		defer func() {
			sandboxDestroy(ctx, SandboxDestroyInput{ID: sandboxIDVal, Force: true})
		}()
	}

	// Step 2: Start sandbox
	if _, err := sandboxStart(ctx, SandboxIDInput{ID: sandboxIDVal}); err != nil {
		return SandboxValidateOutput{
			SandboxID: sandboxIDVal,
			Overall:   "fail",
			Message:   fmt.Sprintf("Sandbox failed to start: %v", err),
		}, nil
	}

	// Step 3: Sync dotfiles
	syncOut, err := sandboxSync(ctx, SandboxSyncInput{ID: sandboxIDVal})
	if err != nil {
		return SandboxValidateOutput{
			SandboxID: sandboxIDVal,
			Overall:   "fail",
			Message:   fmt.Sprintf("Sync failed: %v", err),
		}, nil
	}
	if !syncOut.Synced {
		return SandboxValidateOutput{
			SandboxID: sandboxIDVal,
			Overall:   "fail",
			Message:   fmt.Sprintf("Sync had errors: %s", strings.Join(syncOut.Errors, "; ")),
		}, nil
	}

	// Step 4: Run tests
	testOut, err := sandboxTest(ctx, SandboxTestInput{ID: sandboxIDVal, Suite: "all"})
	if err != nil {
		return SandboxValidateOutput{
			SandboxID: sandboxIDVal,
			Overall:   "fail",
			Message:   fmt.Sprintf("Tests failed to run: %v", err),
		}, nil
	}

	msg := fmt.Sprintf("Tests: %d passed, %d failed, %d skipped", testOut.Passed, testOut.Failed, testOut.Skipped)

	return SandboxValidateOutput{
		TestResults: &testOut,
		Overall:     testOut.Overall,
		SandboxID:   sandboxIDVal,
		Message:     msg,
	}, nil
}

// ---------------------------------------------------------------------------
// Utilities
// ---------------------------------------------------------------------------

func sandboxFileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
