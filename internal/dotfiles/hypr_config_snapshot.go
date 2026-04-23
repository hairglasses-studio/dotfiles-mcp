// hypr_config_snapshot.go — capture and roll back Hyprland config state.
//
// Sibling to hypr_persistence.go (layouts + monitor presets). This file
// handles the config files themselves: hyprland.conf, monitors.conf, and a
// handful of sibling *.conf pieces loaded by the main file. Snapshots land
// under $XDG_STATE_HOME/dotfiles/desktop-control/hypr/config-snapshots/.
//
// Workflow:
//  1. hypr_config_snapshot            → write-point before a risky edit
//  2. hypr_config_list                → catalog
//  3. hypr_config_rollback            → atomic restore + reload + error check
//
// Rollback is defensive: it restores, reloads, and reads configerrors. If
// errors reappear, the previous state is restored and the tool reports
// applied=false so the caller knows the snapshot was bad.
package dotfiles

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
)

// configSnapshotFiles is the allowlist of hypr config pieces we capture.
// Each entry is a path relative to ~/.config/hypr/. Missing files are
// skipped silently — not every workstation has local.conf or plugin-binds.
var configSnapshotFiles = []string{
	"hyprland.conf",
	"monitors.conf",
	"local.conf",
	"colors.conf",
	"darkwindow-shaders.conf",
	"plugin-binds.conf",
	"hyprshade.toml",
	"wallpaper.env",
}

type hyprConfigSnapshotInput struct {
	Name string `json:"name,omitempty" jsonschema:"description=Label for the snapshot. Used as a suffix on the directory name. Defaults to 'manual'."`
}

type hyprConfigSnapshotOutput struct {
	// Name is the directory name of the snapshot — the full identifier
	// callers pass to hypr_config_rollback. Includes the timestamp prefix
	// plus the user-supplied label.
	Name string `json:"name"`
	// Label is the user-supplied label component (sanitized), without the
	// timestamp prefix. Useful for grouping snapshots that share a reason.
	Label   string   `json:"label"`
	Path    string   `json:"path"`
	SavedAt string   `json:"saved_at"`
	Files   []string `json:"files"`
	GitSHA  string   `json:"git_sha,omitempty"`
	Kernel  string   `json:"kernel,omitempty"`
	Driver  string   `json:"driver,omitempty"`
	HyprVer string   `json:"hypr_version,omitempty"`
}

type hyprConfigListInput struct{}

type hyprConfigListEntry struct {
	Name    string   `json:"name"`
	Path    string   `json:"path"`
	SavedAt string   `json:"saved_at"`
	Files   []string `json:"files"`
	GitSHA  string   `json:"git_sha,omitempty"`
	Kernel  string   `json:"kernel,omitempty"`
	Driver  string   `json:"driver,omitempty"`
}

type hyprConfigListOutput struct {
	Snapshots []hyprConfigListEntry `json:"snapshots"`
}

type hyprConfigRollbackInput struct {
	Name   string `json:"name" jsonschema:"required,description=Snapshot directory name (see hypr_config_list). Use 'latest' to restore the most recent snapshot."`
	DryRun bool   `json:"dry_run,omitempty" jsonschema:"description=Show which files would be restored without writing anything."`
}

type hyprConfigRollbackOutput struct {
	Applied          bool     `json:"applied"`
	SnapshotName     string   `json:"snapshot_name"`
	FilesRestored    []string `json:"files_restored,omitempty"`
	ReloadErrors     []string `json:"reload_errors,omitempty"`
	RolledForward    bool     `json:"rolled_forward,omitempty"`
	RollForwardNotes []string `json:"roll_forward_notes,omitempty"`
	DryRun           bool     `json:"dry_run,omitempty"`
}

// hyprConfigSnapshotRoot returns the root directory for all snapshots.
func hyprConfigSnapshotRoot() (string, error) {
	return ensureDotfilesManagedStateDir("hypr", "config-snapshots")
}

// hyprConfigLiveDir returns the live ~/.config/hypr directory.
func hyprConfigLiveDir() string {
	return filepath.Join(homeDir(), ".config", "hypr")
}

// hyprConfigPersistenceTools returns the tool definitions for this module.
// Registered via mod_hyprland.go (HyprlandModule).
func hyprConfigPersistenceTools() []registry.ToolDefinition {
	return []registry.ToolDefinition{
		handler.TypedHandler[hyprConfigSnapshotInput, hyprConfigSnapshotOutput](
			"hypr_config_snapshot",
			"Capture the current Hyprland config (hyprland.conf, monitors.conf, and sibling *.conf pieces) to a timestamped directory with meta.json. Use before a risky edit so hypr_config_rollback has something to restore.",
			func(_ context.Context, input hyprConfigSnapshotInput) (hyprConfigSnapshotOutput, error) {
				return hyprConfigSnapshotCreate(input.Name)
			},
		),
		handler.TypedHandler[hyprConfigListInput, hyprConfigListOutput](
			"hypr_config_list",
			"List all Hyprland config snapshots with their metadata (git SHA, kernel, driver) in newest-first order.",
			func(_ context.Context, _ hyprConfigListInput) (hyprConfigListOutput, error) {
				return hyprConfigSnapshotList()
			},
		),
		handler.TypedHandler[hyprConfigRollbackInput, hyprConfigRollbackOutput](
			"hypr_config_rollback",
			"Restore a previous Hyprland config snapshot atomically, reload the compositor, and check configerrors. If errors reappear, the prior live state is restored and applied=false is returned so the caller knows the snapshot itself was bad.",
			func(_ context.Context, input hyprConfigRollbackInput) (hyprConfigRollbackOutput, error) {
				return hyprConfigSnapshotRollback(input.Name, input.DryRun)
			},
		),
	}
}

// ---------------------------------------------------------------------------
// Snapshot
// ---------------------------------------------------------------------------

func hyprConfigSnapshotCreate(name string) (hyprConfigSnapshotOutput, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "manual"
	}
	sanitized := safeStateName(name)

	root, err := hyprConfigSnapshotRoot()
	if err != nil {
		return hyprConfigSnapshotOutput{}, err
	}

	now := time.Now().UTC()
	dirName := fmt.Sprintf("%s_%s", now.Format("20060102-150405"), sanitized)
	dir := filepath.Join(root, dirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return hyprConfigSnapshotOutput{}, fmt.Errorf("create snapshot dir: %w", err)
	}

	live := hyprConfigLiveDir()
	var captured []string
	for _, rel := range configSnapshotFiles {
		src := filepath.Join(live, rel)
		info, err := os.Stat(src)
		if err != nil || info.IsDir() {
			continue
		}
		dst := filepath.Join(dir, rel)
		if err := copyFileAtomic(src, dst, info.Mode()); err != nil {
			return hyprConfigSnapshotOutput{}, fmt.Errorf("copy %s: %w", rel, err)
		}
		captured = append(captured, rel)
	}

	meta := hyprConfigSnapshotOutput{
		Name:    dirName,
		Label:   sanitized,
		Path:    dir,
		SavedAt: now.Format(time.RFC3339),
		Files:   captured,
		GitSHA:  captureGitSHA(),
		Kernel:  captureKernel(),
		Driver:  captureDriverVersion(),
		HyprVer: captureHyprVersion(),
	}

	metaPath := filepath.Join(dir, "meta.json")
	data, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.WriteFile(metaPath, data, 0o644); err != nil {
		return hyprConfigSnapshotOutput{}, fmt.Errorf("write meta.json: %w", err)
	}

	return meta, nil
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

func hyprConfigSnapshotList() (hyprConfigListOutput, error) {
	root, err := hyprConfigSnapshotRoot()
	if err != nil {
		return hyprConfigListOutput{}, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return hyprConfigListOutput{}, nil
		}
		return hyprConfigListOutput{}, err
	}
	var out hyprConfigListOutput
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		entry := hyprConfigListEntry{Name: e.Name(), Path: dir}
		if meta, err := readSnapshotMeta(dir); err == nil {
			entry.SavedAt = meta.SavedAt
			entry.Files = meta.Files
			entry.GitSHA = meta.GitSHA
			entry.Kernel = meta.Kernel
			entry.Driver = meta.Driver
		}
		out.Snapshots = append(out.Snapshots, entry)
	}
	// Newest first by directory name (timestamp prefix sorts lexicographically).
	sort.Slice(out.Snapshots, func(i, j int) bool {
		return out.Snapshots[i].Name > out.Snapshots[j].Name
	})
	return out, nil
}

// ---------------------------------------------------------------------------
// Rollback
// ---------------------------------------------------------------------------

func hyprConfigSnapshotRollback(name string, dryRun bool) (hyprConfigRollbackOutput, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return hyprConfigRollbackOutput{}, fmt.Errorf("[%s] name is required", handler.ErrInvalidParam)
	}
	out := hyprConfigRollbackOutput{DryRun: dryRun}

	root, err := hyprConfigSnapshotRoot()
	if err != nil {
		return out, err
	}

	// Resolve "latest" to the most recent snapshot.
	if name == "latest" {
		list, err := hyprConfigSnapshotList()
		if err != nil {
			return out, err
		}
		if len(list.Snapshots) == 0 {
			return out, fmt.Errorf("[%s] no snapshots exist to roll back to", handler.ErrInvalidParam)
		}
		name = list.Snapshots[0].Name
	}
	out.SnapshotName = name

	src := filepath.Join(root, name)
	info, err := os.Stat(src)
	if err != nil || !info.IsDir() {
		return out, fmt.Errorf("[%s] snapshot %q not found under %s", handler.ErrInvalidParam, name, root)
	}

	live := hyprConfigLiveDir()

	// In dry-run, report which files would change and exit.
	if dryRun {
		for _, rel := range configSnapshotFiles {
			if _, err := os.Stat(filepath.Join(src, rel)); err == nil {
				out.FilesRestored = append(out.FilesRestored, rel)
			}
		}
		return out, nil
	}

	// Take a pre-rollback safety snapshot so we can roll forward if the
	// snapshot itself is bad.
	preRollback, snapErr := hyprConfigSnapshotCreate("pre-rollback-" + name)
	if snapErr != nil {
		// Non-fatal — proceed, but we cannot auto roll-forward on error.
		preRollback = hyprConfigSnapshotOutput{}
	}

	for _, rel := range configSnapshotFiles {
		srcFile := filepath.Join(src, rel)
		info, err := os.Stat(srcFile)
		if err != nil || info.IsDir() {
			continue
		}
		dstFile := filepath.Join(live, rel)
		if err := copyFileAtomic(srcFile, dstFile, info.Mode()); err != nil {
			return out, fmt.Errorf("restore %s: %w", rel, err)
		}
		out.FilesRestored = append(out.FilesRestored, rel)
	}

	// Reload + verify. If errors reappear and we have a safety snapshot,
	// roll forward so the user is not stuck in a worse state than they
	// started.
	_, _ = runHyprctl("reload")
	raw, configErr := hyprQueryMaybeJSON("configerrors")
	var errMsgs []string
	if configErr == nil {
		errMsgs = hyprConfigErrorMessages(raw)
	}
	out.ReloadErrors = errMsgs

	if len(errMsgs) > 0 && preRollback.Path != "" {
		// Snapshot was bad or incompatible — restore what was live before.
		rollForward, err := hyprConfigSnapshotRollbackToDir(preRollback.Path, live)
		out.RolledForward = true
		if err != nil {
			out.RollForwardNotes = append(out.RollForwardNotes,
				fmt.Sprintf("roll-forward partially failed: %v", err))
		}
		out.RollForwardNotes = append(out.RollForwardNotes, rollForward...)
		out.Applied = false
		_, _ = runHyprctl("reload")
		return out, nil
	}

	out.Applied = true
	return out, nil
}

// hyprConfigSnapshotRollbackToDir is the raw restore half of rollback,
// without the reload/verify step. Used to unwind a bad rollback.
func hyprConfigSnapshotRollbackToDir(src, live string) ([]string, error) {
	var restored []string
	for _, rel := range configSnapshotFiles {
		srcFile := filepath.Join(src, rel)
		info, err := os.Stat(srcFile)
		if err != nil || info.IsDir() {
			continue
		}
		dstFile := filepath.Join(live, rel)
		if err := copyFileAtomic(srcFile, dstFile, info.Mode()); err != nil {
			return restored, fmt.Errorf("restore %s: %w", rel, err)
		}
		restored = append(restored, rel)
	}
	return restored, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// copyFileAtomic writes dst via a temp file in the same directory and
// renames it into place. Fails on any error so callers can surface it.
func copyFileAtomic(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp, err := os.CreateTemp(filepath.Dir(dst), ".snap-")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		// Best-effort cleanup if rename didn't happen.
		_ = os.Remove(tmpName)
	}()

	if _, err := io.Copy(tmp, in); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, dst)
}

func readSnapshotMeta(dir string) (hyprConfigSnapshotOutput, error) {
	var m hyprConfigSnapshotOutput
	data, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		return m, err
	}
	err = json.Unmarshal(data, &m)
	return m, err
}

func captureGitSHA() string {
	cmd := exec.Command("git", "-C", dotfilesDir(), "rev-parse", "--short", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func captureKernel() string {
	data, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

var driverVersionRe = regexp.MustCompile(`(\d+\.\d+(?:\.\d+)?)`)

func captureDriverVersion() string {
	data, err := os.ReadFile("/proc/driver/nvidia/version")
	if err != nil {
		return ""
	}
	// Expected format: "NVRM version: NVIDIA UNIX x86_64 Kernel Module  590.48.01  Mon Apr 14 ...".
	line := strings.SplitN(string(data), "\n", 2)[0]
	if m := driverVersionRe.FindString(line); m != "" {
		return m
	}
	return ""
}

func captureHyprVersion() string {
	out, err := runHyprctl("version")
	if err != nil {
		return ""
	}
	// First non-empty line is "Hyprland, built from branch ..." — keep it short.
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}
