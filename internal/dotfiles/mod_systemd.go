// mod_systemd.go — systemd service and timer management tools via systemctl/journalctl
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// systemdRunCmd executes a command and returns stdout, stderr, error.
func systemdRunCmd(ctx context.Context, name string, args ...string) (string, string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.Output()
	stdout := string(out)
	var stderr string
	if exitErr, ok := err.(*exec.ExitError); ok {
		stderr = string(exitErr.Stderr)
	}
	return stdout, stderr, err
}

// systemdRunSystemctl runs systemctl with optional --user flag.
func systemdRunSystemctl(ctx context.Context, user bool, args ...string) (string, error) {
	cmdArgs := make([]string, 0, len(args)+1)
	if user {
		cmdArgs = append(cmdArgs, "--user")
	}
	cmdArgs = append(cmdArgs, args...)
	stdout, stderr, err := systemdRunCmd(ctx, "systemctl", cmdArgs...)
	if err != nil {
		return "", fmt.Errorf("systemctl %s: %s: %w", strings.Join(cmdArgs, " "), stderr, err)
	}
	return stdout, nil
}

// systemdRunJournalctl runs journalctl with the appropriate unit flag.
func systemdRunJournalctl(ctx context.Context, user bool, args ...string) (string, error) {
	cmdArgs := make([]string, 0, len(args)+1)
	if user {
		cmdArgs = append(cmdArgs, "--user-unit")
	} else {
		cmdArgs = append(cmdArgs, "-u")
	}
	cmdArgs = append(cmdArgs, args...)
	stdout, stderr, err := systemdRunCmd(ctx, "journalctl", cmdArgs...)
	if err != nil {
		return "", fmt.Errorf("journalctl: %s: %w", stderr, err)
	}
	return stdout, nil
}

// ---------------------------------------------------------------------------
// I/O types
// ---------------------------------------------------------------------------

// ── systemd_status ─────────────────────────────────────────────────────────

type SystemdStatusInput struct {
	Unit   string `json:"unit" jsonschema:"required,description=Systemd unit name (e.g. makima.service or shader-rotate.timer)"`
	System bool   `json:"system,omitempty" jsonschema:"description=Target system scope instead of user scope. Default: user scope."`
}

type SystemdStatusOutput struct {
	Unit                 string `json:"unit"`
	ActiveState          string `json:"active_state"`
	SubState             string `json:"sub_state"`
	Description          string `json:"description"`
	LoadState            string `json:"load_state"`
	FragmentPath         string `json:"fragment_path,omitempty"`
	ActiveEnterTimestamp string `json:"active_enter_timestamp,omitempty"`
	MainPID              int    `json:"main_pid,omitempty"`
	MemoryCurrent        string `json:"memory_current,omitempty"`
	CPUUsageNSec         string `json:"cpu_usage_nsec,omitempty"`
}

// ── systemd_start ──────────────────────────────────────────────────────────

type SystemdStartInput struct {
	Unit   string `json:"unit" jsonschema:"required,description=Systemd unit name to start"`
	System bool   `json:"system,omitempty" jsonschema:"description=Target system scope instead of user scope. Default: user scope."`
}

type SystemdStartOutput struct {
	Unit    string `json:"unit"`
	Message string `json:"message"`
}

// ── systemd_stop ───────────────────────────────────────────────────────────

type SystemdStopInput struct {
	Unit    string `json:"unit" jsonschema:"required,description=Systemd unit name to stop"`
	System  bool   `json:"system,omitempty" jsonschema:"description=Target system scope instead of user scope. Default: user scope."`
	Confirm bool   `json:"confirm,omitempty" jsonschema:"description=Required for critical services (sshd, NetworkManager, systemd-*, dbus, polkit)"`
}

type SystemdStopOutput struct {
	Unit    string `json:"unit"`
	Message string `json:"message"`
}

// ── systemd_restart ────────────────────────────────────────────────────────

type SystemdRestartInput struct {
	Unit   string `json:"unit" jsonschema:"required,description=Systemd unit name to restart"`
	System bool   `json:"system,omitempty" jsonschema:"description=Target system scope instead of user scope. Default: user scope."`
}

type SystemdRestartOutput struct {
	Unit    string `json:"unit"`
	Message string `json:"message"`
}

// ── systemd_enable ─────────────────────────────────────────────────────────

type SystemdEnableInput struct {
	Unit   string `json:"unit" jsonschema:"required,description=Systemd unit name to enable"`
	System bool   `json:"system,omitempty" jsonschema:"description=Target system scope instead of user scope. Default: user scope."`
}

type SystemdEnableOutput struct {
	Unit    string `json:"unit"`
	Message string `json:"message"`
}

// ── systemd_disable ────────────────────────────────────────────────────────

type SystemdDisableInput struct {
	Unit    string `json:"unit" jsonschema:"required,description=Systemd unit name to disable"`
	System  bool   `json:"system,omitempty" jsonschema:"description=Target system scope instead of user scope. Default: user scope."`
	Confirm bool   `json:"confirm,omitempty" jsonschema:"description=Required for critical services (sshd, NetworkManager, systemd-*, dbus, polkit)"`
}

type SystemdDisableOutput struct {
	Unit    string `json:"unit"`
	Message string `json:"message"`
}

// ── systemd_logs ───────────────────────────────────────────────────────────

type SystemdLogsInput struct {
	Unit   string `json:"unit" jsonschema:"required,description=Systemd unit name to fetch logs for"`
	Lines  int    `json:"lines,omitempty" jsonschema:"description=Number of log lines to return. Default 50."`
	Since  string `json:"since,omitempty" jsonschema:"description=Show logs since this time (e.g. '1h ago' or '2024-01-01')"`
	System bool   `json:"system,omitempty" jsonschema:"description=Target system scope instead of user scope. Default: user scope."`
}

type SystemdLogsOutput struct {
	Unit  string `json:"unit"`
	Lines int    `json:"lines"`
	Logs  string `json:"logs"`
}

// ── systemd_list_units ─────────────────────────────────────────────────────

type SystemdListUnitsInput struct {
	State  string `json:"state,omitempty" jsonschema:"description=Filter by unit state (e.g. active, inactive, failed)"`
	System bool   `json:"system,omitempty" jsonschema:"description=Target system scope instead of user scope. Default: user scope."`
}

type SystemdListUnitsOutput struct {
	Units json.RawMessage `json:"units"`
}

// ── systemd_list_timers ────────────────────────────────────────────────────

type SystemdListTimersInput struct {
	System bool `json:"system,omitempty" jsonschema:"description=Target system scope instead of user scope. Default: user scope."`
}

type SystemdListTimersOutput struct {
	Timers json.RawMessage `json:"timers"`
}

// ── systemd_failed ─────────────────────────────────────────────────────────

type SystemdFailedInput struct {
	System bool `json:"system,omitempty" jsonschema:"description=Target system scope instead of user scope. Default: user scope."`
}

type SystemdFailedOutput struct {
	Units json.RawMessage `json:"units"`
}

// ── systemd_restart_verify (composed) ──────────────────────────────────────

type SystemdRestartVerifyInput struct {
	Unit      string `json:"unit" jsonschema:"required,description=systemd unit name (e.g. logid.service)"`
	TimeoutMs int    `json:"timeout_ms,omitempty" jsonschema:"description=Max time to wait for unit to become active (default 10000ms)"`
	Scope     string `json:"scope,omitempty" jsonschema:"description=systemd scope: user (default) or system,enum=user,enum=system"`
}

type SystemdRestartVerifyOutput struct {
	Unit          string   `json:"unit"`
	PreviousState string   `json:"previous_state"`
	NewState      string   `json:"new_state"`
	Success       bool     `json:"success"`
	DurationMs    int64    `json:"duration_ms"`
	Logs          []string `json:"logs"`
}

// ---------------------------------------------------------------------------
// Critical service guard
// ---------------------------------------------------------------------------

// systemdCriticalPrefixes lists service name prefixes that require explicit
// confirmation before being stopped or disabled.
var systemdCriticalPrefixes = []string{"sshd", "NetworkManager", "systemd-", "dbus", "polkit"}

func systemdRequireConfirmation(unit string, confirm bool, action string) error {
	for _, prefix := range systemdCriticalPrefixes {
		if strings.HasPrefix(unit, prefix) && !confirm {
			return fmt.Errorf("[%s] %sing critical service %q requires confirm: true",
				handler.ErrInvalidParam, action, unit)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Module
// ---------------------------------------------------------------------------

// SystemdModule provides systemd service and timer management tools.
type SystemdModule struct{}

func (m *SystemdModule) Name() string { return "systemd" }
func (m *SystemdModule) Description() string {
	return "systemd service management: status, start/stop/restart, logs, units, timers"
}

func (m *SystemdModule) Tools() []registry.ToolDefinition {
	// ── Read-only tools ────────────────────────────────────────────────

	status := handler.TypedHandler[SystemdStatusInput, SystemdStatusOutput](
		"systemd_status",
		"Show detailed status of a systemd unit including active state, PID, memory, and CPU usage.",
		func(ctx context.Context, input SystemdStatusInput) (SystemdStatusOutput, error) {
			user := !input.System
			out, err := systemdRunSystemctl(ctx, user, "show",
				"--property=ActiveState,SubState,Description,LoadState,FragmentPath,ActiveEnterTimestamp,MainPID,MemoryCurrent,CPUUsageNSec",
				input.Unit,
			)
			if err != nil {
				return SystemdStatusOutput{}, fmt.Errorf("[%s] %w", handler.ErrNotFound, err)
			}

			result := SystemdStatusOutput{Unit: input.Unit}
			for line := range strings.SplitSeq(out, "\n") {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) != 2 {
					continue
				}
				key, val := parts[0], parts[1]
				switch key {
				case "ActiveState":
					result.ActiveState = val
				case "SubState":
					result.SubState = val
				case "Description":
					result.Description = val
				case "LoadState":
					result.LoadState = val
				case "FragmentPath":
					result.FragmentPath = val
				case "ActiveEnterTimestamp":
					result.ActiveEnterTimestamp = val
				case "MainPID":
					result.MainPID, _ = strconv.Atoi(val)
				case "MemoryCurrent":
					if val != "[not set]" {
						result.MemoryCurrent = val
					}
				case "CPUUsageNSec":
					if val != "[not set]" {
						result.CPUUsageNSec = val
					}
				}
			}

			if result.LoadState == "not-found" {
				return result, fmt.Errorf("[%s] unit %s not found", handler.ErrNotFound, input.Unit)
			}

			return result, nil
		},
	)
	status.Category = "systemd"
	status.SearchTerms = []string{"service status", "unit status", "systemd", "systemctl"}

	listUnits := handler.TypedHandler[SystemdListUnitsInput, SystemdListUnitsOutput](
		"systemd_list_units",
		"List systemd units, optionally filtered by state.",
		func(ctx context.Context, input SystemdListUnitsInput) (SystemdListUnitsOutput, error) {
			user := !input.System
			args := []string{"list-units", "--output=json"}
			if input.State != "" {
				args = append(args, "--state="+input.State)
			}
			out, err := systemdRunSystemctl(ctx, user, args...)
			if err != nil {
				return SystemdListUnitsOutput{}, fmt.Errorf("[%s] %w", handler.ErrUpstreamError, err)
			}
			return SystemdListUnitsOutput{
				Units: json.RawMessage(out),
			}, nil
		},
	)
	listUnits.Category = "systemd"
	listUnits.SearchTerms = []string{"list services", "list units", "systemd units"}

	listTimers := handler.TypedHandler[SystemdListTimersInput, SystemdListTimersOutput](
		"systemd_list_timers",
		"List active systemd timers with their next/last trigger times.",
		func(ctx context.Context, input SystemdListTimersInput) (SystemdListTimersOutput, error) {
			user := !input.System
			out, err := systemdRunSystemctl(ctx, user, "list-timers", "--output=json", "--no-pager")
			if err != nil {
				return SystemdListTimersOutput{}, fmt.Errorf("[%s] %w", handler.ErrUpstreamError, err)
			}
			return SystemdListTimersOutput{
				Timers: json.RawMessage(out),
			}, nil
		},
	)
	listTimers.Category = "systemd"
	listTimers.SearchTerms = []string{"list timers", "cron", "scheduled", "timer"}

	logs := handler.TypedHandler[SystemdLogsInput, SystemdLogsOutput](
		"systemd_logs",
		"Fetch recent journal logs for a systemd unit.",
		func(ctx context.Context, input SystemdLogsInput) (SystemdLogsOutput, error) {
			user := !input.System
			lines := input.Lines
			if lines <= 0 {
				lines = 50
			}

			args := []string{input.Unit, "-n", strconv.Itoa(lines)}
			if input.Since != "" {
				args = append(args, "--since", input.Since)
			}
			args = append(args, "--no-pager")

			out, err := systemdRunJournalctl(ctx, user, args...)
			if err != nil {
				return SystemdLogsOutput{}, fmt.Errorf("[%s] %w", handler.ErrUpstreamError, err)
			}
			return SystemdLogsOutput{
				Unit:  input.Unit,
				Lines: lines,
				Logs:  out,
			}, nil
		},
	)
	logs.Category = "systemd"
	logs.SearchTerms = []string{"journal", "journald", "service logs", "unit logs"}
	logs.MaxResultChars = 8000

	failed := handler.TypedHandler[SystemdFailedInput, SystemdFailedOutput](
		"systemd_failed",
		"List failed systemd units.",
		func(ctx context.Context, input SystemdFailedInput) (SystemdFailedOutput, error) {
			user := !input.System
			out, err := systemdRunSystemctl(ctx, user, "--failed", "--output=json", "--no-pager")
			if err != nil {
				return SystemdFailedOutput{}, fmt.Errorf("[%s] %w", handler.ErrUpstreamError, err)
			}
			return SystemdFailedOutput{
				Units: json.RawMessage(out),
			}, nil
		},
	)
	failed.Category = "systemd"
	failed.SearchTerms = []string{"failed services", "broken units", "systemd errors"}

	// ── Mutating tools (IsWrite: true) ─────────────────────────────────

	start := handler.TypedHandler[SystemdStartInput, SystemdStartOutput](
		"systemd_start",
		"Start a systemd unit.",
		func(ctx context.Context, input SystemdStartInput) (SystemdStartOutput, error) {
			slog.Info("starting unit", "unit", input.Unit, "system", input.System)
			user := !input.System
			_, err := systemdRunSystemctl(ctx, user, "start", input.Unit)
			if err != nil {
				slog.Error("unit start failed", "unit", input.Unit, "error", err)
				return SystemdStartOutput{}, fmt.Errorf("[%s] %w", handler.ErrUpstreamError, err)
			}
			slog.Info("unit started", "unit", input.Unit)
			return SystemdStartOutput{
				Unit:    input.Unit,
				Message: input.Unit + " started",
			}, nil
		},
	)
	start.IsWrite = true
	start.Complexity = registry.ComplexityModerate
	start.Category = "systemd"
	start.SearchTerms = []string{"start service", "start unit"}

	restart := handler.TypedHandler[SystemdRestartInput, SystemdRestartOutput](
		"systemd_restart",
		"Restart a systemd unit.",
		func(ctx context.Context, input SystemdRestartInput) (SystemdRestartOutput, error) {
			slog.Info("restarting unit", "unit", input.Unit, "system", input.System)
			user := !input.System
			_, err := systemdRunSystemctl(ctx, user, "restart", input.Unit)
			if err != nil {
				slog.Error("unit restart failed", "unit", input.Unit, "error", err)
				return SystemdRestartOutput{}, fmt.Errorf("[%s] %w", handler.ErrUpstreamError, err)
			}
			slog.Info("unit restarted", "unit", input.Unit)
			return SystemdRestartOutput{
				Unit:    input.Unit,
				Message: input.Unit + " restarted",
			}, nil
		},
	)
	restart.IsWrite = true
	restart.Complexity = registry.ComplexityModerate
	restart.Category = "systemd"
	restart.SearchTerms = []string{"restart service", "restart unit"}

	enable := handler.TypedHandler[SystemdEnableInput, SystemdEnableOutput](
		"systemd_enable",
		"Enable a systemd unit to start on boot/login.",
		func(ctx context.Context, input SystemdEnableInput) (SystemdEnableOutput, error) {
			slog.Info("enabling unit", "unit", input.Unit, "system", input.System)
			user := !input.System
			_, err := systemdRunSystemctl(ctx, user, "enable", input.Unit)
			if err != nil {
				slog.Error("unit enable failed", "unit", input.Unit, "error", err)
				return SystemdEnableOutput{}, fmt.Errorf("[%s] %w", handler.ErrUpstreamError, err)
			}
			slog.Info("unit enabled", "unit", input.Unit)
			return SystemdEnableOutput{
				Unit:    input.Unit,
				Message: input.Unit + " enabled",
			}, nil
		},
	)
	enable.IsWrite = true
	enable.Complexity = registry.ComplexityModerate
	enable.Category = "systemd"
	enable.SearchTerms = []string{"enable service", "enable unit", "autostart"}

	// ── Destructive tools (IsWrite: true, ComplexityComplex) ───────────

	stop := handler.TypedHandler[SystemdStopInput, SystemdStopOutput](
		"systemd_stop",
		"Stop a systemd unit. Critical services (sshd, NetworkManager, systemd-*, dbus, polkit) require confirm: true.",
		func(ctx context.Context, input SystemdStopInput) (SystemdStopOutput, error) {
			if err := systemdRequireConfirmation(input.Unit, input.Confirm, "stopp"); err != nil {
				return SystemdStopOutput{}, err
			}

			slog.Info("stopping unit", "unit", input.Unit, "system", input.System)
			user := !input.System
			_, err := systemdRunSystemctl(ctx, user, "stop", input.Unit)
			if err != nil {
				slog.Error("unit stop failed", "unit", input.Unit, "error", err)
				return SystemdStopOutput{}, fmt.Errorf("[%s] %w", handler.ErrUpstreamError, err)
			}
			slog.Info("unit stopped", "unit", input.Unit)
			return SystemdStopOutput{
				Unit:    input.Unit,
				Message: input.Unit + " stopped",
			}, nil
		},
	)
	stop.IsWrite = true
	stop.Complexity = registry.ComplexityComplex
	stop.Category = "systemd"
	stop.SearchTerms = []string{"stop service", "stop unit", "kill service"}

	disable := handler.TypedHandler[SystemdDisableInput, SystemdDisableOutput](
		"systemd_disable",
		"Disable a systemd unit from starting on boot/login. Critical services (sshd, NetworkManager, systemd-*, dbus, polkit) require confirm: true.",
		func(ctx context.Context, input SystemdDisableInput) (SystemdDisableOutput, error) {
			if err := systemdRequireConfirmation(input.Unit, input.Confirm, "disabl"); err != nil {
				return SystemdDisableOutput{}, err
			}

			slog.Info("disabling unit", "unit", input.Unit, "system", input.System)
			user := !input.System
			_, err := systemdRunSystemctl(ctx, user, "disable", input.Unit)
			if err != nil {
				slog.Error("unit disable failed", "unit", input.Unit, "error", err)
				return SystemdDisableOutput{}, fmt.Errorf("[%s] %w", handler.ErrUpstreamError, err)
			}
			slog.Info("unit disabled", "unit", input.Unit)
			return SystemdDisableOutput{
				Unit:    input.Unit,
				Message: input.Unit + " disabled",
			}, nil
		},
	)
	disable.IsWrite = true
	disable.Complexity = registry.ComplexityComplex
	disable.Category = "systemd"
	disable.SearchTerms = []string{"disable service", "disable unit", "remove autostart"}

	// ── Composed workflows ─────────────────────────────────────────────

	restartVerify := handler.TypedHandler[SystemdRestartVerifyInput, SystemdRestartVerifyOutput](
		"systemd_restart_verify",
		"Composed workflow: restart a systemd unit, poll until active (or timeout), and return previous state, new state, and recent logs. Eliminates the multi-step restart→status→logs dance.",
		func(ctx context.Context, input SystemdRestartVerifyInput) (SystemdRestartVerifyOutput, error) {
			user := input.Scope != "system"
			timeoutMs := input.TimeoutMs
			if timeoutMs <= 0 {
				timeoutMs = 10000
			}

			slog.Info("restart-verify: starting", "unit", input.Unit, "scope", input.Scope, "timeout_ms", timeoutMs)

			// Step 1: capture previous state
			prevOut, err := systemdRunSystemctl(ctx, user, "is-active", input.Unit)
			previousState := strings.TrimSpace(prevOut)
			if err != nil {
				// is-active returns exit 3 for inactive/failed — that's fine, we still get stdout
				if previousState == "" {
					previousState = "unknown"
				}
			}

			// Step 2: restart the unit
			startTime := time.Now()
			_, err = systemdRunSystemctl(ctx, user, "restart", input.Unit)
			if err != nil {
				slog.Error("restart-verify: restart failed", "unit", input.Unit, "error", err)
				return SystemdRestartVerifyOutput{
					Unit:          input.Unit,
					PreviousState: previousState,
					NewState:      "restart-failed",
					Success:       false,
					DurationMs:    time.Since(startTime).Milliseconds(),
				}, fmt.Errorf("[%s] restart failed: %w", handler.ErrUpstreamError, err)
			}

			// Step 3: poll is-active every 500ms until active or timeout
			deadline := time.After(time.Duration(timeoutMs) * time.Millisecond)
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()

			var newState string
			for {
				select {
				case <-ctx.Done():
					return SystemdRestartVerifyOutput{
						Unit:          input.Unit,
						PreviousState: previousState,
						NewState:      "context-cancelled",
						Success:       false,
						DurationMs:    time.Since(startTime).Milliseconds(),
					}, ctx.Err()
				case <-deadline:
					// Final check before giving up
					out, _ := systemdRunSystemctl(ctx, user, "is-active", input.Unit)
					newState = strings.TrimSpace(out)
					if newState == "" {
						newState = "timeout"
					}
					goto done
				case <-ticker.C:
					out, _ := systemdRunSystemctl(ctx, user, "is-active", input.Unit)
					newState = strings.TrimSpace(out)
					if newState == "active" {
						goto done
					}
				}
			}

		done:
			durationMs := time.Since(startTime).Milliseconds()
			success := newState == "active"

			slog.Info("restart-verify: completed",
				"unit", input.Unit,
				"previous", previousState,
				"new", newState,
				"success", success,
				"duration_ms", durationMs,
			)

			// Step 4: fetch last 20 log lines
			logArgs := []string{input.Unit, "-n", "20", "--no-pager"}
			logOut, logErr := systemdRunJournalctl(ctx, user, logArgs...)
			var logLines []string
			if logErr == nil {
				for _, line := range strings.Split(strings.TrimSpace(logOut), "\n") {
					if line != "" {
						logLines = append(logLines, line)
					}
				}
			} else {
				logLines = []string{fmt.Sprintf("(failed to fetch logs: %v)", logErr)}
			}

			return SystemdRestartVerifyOutput{
				Unit:          input.Unit,
				PreviousState: previousState,
				NewState:      newState,
				Success:       success,
				DurationMs:    durationMs,
				Logs:          logLines,
			}, nil
		},
	)
	restartVerify.IsWrite = true
	restartVerify.Complexity = registry.ComplexityComplex
	restartVerify.Category = "systemd"
	restartVerify.SearchTerms = []string{"restart verify", "restart check", "restart poll", "composed restart", "service health"}

	return []registry.ToolDefinition{
		status,
		start,
		stop,
		restart,
		enable,
		disable,
		logs,
		listUnits,
		listTimers,
		failed,
		restartVerify,
	}
}
