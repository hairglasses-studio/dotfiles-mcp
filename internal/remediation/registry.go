// Package remediation maps known error conditions to structured, callable
// fixes. Consumers (error handlers, hook scripts, slash commands, the model
// itself) can look up a Remediation by ErrorCode and dispatch the referenced
// tool directly instead of reinterpreting a free-form suggestion string.
//
// The registry is an in-memory, read-only map populated at package init.
// Adding a new remediation: declare a new ErrorCode constant, then append
// to seeds() below. No locking is needed — writes happen only during init.
package remediation

import (
	"sort"
	"sync"
)

// Risk describes how aggressive the remediation is. Consumers that want
// an auto-apply path should only act on RiskSafe entries without explicit
// human confirmation.
type Risk string

const (
	// RiskSafe is idempotent, side-effect-free on success, and fully
	// reversible if it fails (no state left behind).
	RiskSafe Risk = "safe"
	// RiskReload restarts or reloads a service; visible to the user but
	// not destructive. Typical: systemctl restart, hyprctl reload.
	RiskReload Risk = "reload"
	// RiskDestructive mutates persistent state in a way that is not
	// trivially undoable (e.g. rewrites a config file, removes data).
	// Always requires explicit confirmation.
	RiskDestructive Risk = "destructive"
)

// Remediation is a structured, dispatchable fix for a known error class.
type Remediation struct {
	// Tool is the MCP tool name to invoke, e.g. "hypr_reload_config".
	Tool string `json:"tool"`
	// Args is the default argument set; callers may merge/override.
	Args map[string]any `json:"args,omitempty"`
	// Idempotent is true when repeating the call is harmless on success.
	Idempotent bool `json:"idempotent"`
	// Risk indicates how aggressive this action is.
	Risk Risk `json:"risk"`
	// Why is a one-line human-readable explanation for UIs and notifications.
	Why string `json:"why"`
}

// ErrorCode is a short, stable identifier for a known error class.
// Codes are namespaced by subsystem (hypr_*, go_*, systemd_*, etc.) so that
// the catalog stays browsable as it grows.
type ErrorCode string

// Known error codes. Add new entries here, then register them in seeds().
const (
	CodeHyprConfigParse         ErrorCode = "hypr_config_parse"
	CodeHyprConfigErrors        ErrorCode = "hypr_config_errors"
	CodeHyprReloadInducedDrm    ErrorCode = "hypr_reload_induced_drm"
	CodeHyprShaderMissing       ErrorCode = "hypr_shader_missing"
	CodeGoMissingDep            ErrorCode = "go_missing_dep"
	CodeGoMissingImport         ErrorCode = "go_missing_import"
	CodeGoUnusedVar             ErrorCode = "go_unused_var"
	CodeGoLintViolation         ErrorCode = "go_lint_violation"
	CodeGoTimeout               ErrorCode = "go_timeout"
	CodeIronbarStale            ErrorCode = "ironbar_stale"
	CodeTickerStale             ErrorCode = "ticker_stale"
	CodeSwayncDrift             ErrorCode = "swaync_drift"
	CodeMcpkitVersionDrift      ErrorCode = "mcpkit_version_drift"
	CodeSystemdUnitFailed       ErrorCode = "systemd_unit_failed"
	CodePalettePropagationStale ErrorCode = "palette_propagation_stale"
	CodeKittySocketLost         ErrorCode = "kitty_socket_lost"
	CodeMcpServerDead           ErrorCode = "mcp_server_dead"
	CodeAudioSinkLost           ErrorCode = "audio_sink_lost"
)

var (
	registry   map[ErrorCode]Remediation
	registryMu sync.RWMutex
)

func init() {
	registry = seeds()
}

// seeds returns the initial registry contents. Each entry documents why
// the mapping is correct — future maintainers should be able to judge
// whether a proposed remediation is still right without spelunking the
// commit history.
func seeds() map[ErrorCode]Remediation {
	return map[ErrorCode]Remediation{
		CodeHyprConfigParse: {
			Tool:       "hypr_reload_config",
			Idempotent: true,
			Risk:       RiskReload,
			Why:        "Reloads Hyprland to pick up valid syntax after a parse error is corrected.",
		},
		CodeHyprConfigErrors: {
			Tool:       "hypr_reload_config",
			Idempotent: true,
			Risk:       RiskReload,
			Why:        "Reloads the compositor; any remaining errors will reappear in the next get_config_errors call.",
		},
		CodeHyprReloadInducedDrm: {
			Tool:       "hypr_config_rollback",
			Args:       map[string]any{"name": "latest"},
			Idempotent: false,
			Risk:       RiskReload,
			Why:        "A kernel DRM error appeared within 5s of a Hyprland reload — the most recent config change is the likely cause.",
		},
		CodeHyprShaderMissing: {
			Tool:       "shader_random",
			Idempotent: true,
			Risk:       RiskSafe,
			Why:        "Pick any available shader from the playlist so the window has a valid effect.",
		},
		CodeGoMissingDep: {
			Tool:       "ops_auto_fix",
			Args:       map[string]any{"execute": true},
			Idempotent: true,
			Risk:       RiskSafe,
			Why:        "ops_auto_fix runs 'go mod tidy' to add missing module dependencies.",
		},
		CodeGoMissingImport: {
			Tool:       "ops_auto_fix",
			Args:       map[string]any{"execute": true},
			Idempotent: true,
			Risk:       RiskSafe,
			Why:        "ops_auto_fix runs goimports to resolve missing package imports.",
		},
		CodeGoUnusedVar: {
			Tool:       "ops_auto_fix",
			Args:       map[string]any{"execute": true},
			Idempotent: true,
			Risk:       RiskSafe,
			Why:        "ops_auto_fix removes unused variables and imports.",
		},
		CodeGoLintViolation: {
			Tool:       "ops_lint_fix",
			Idempotent: true,
			Risk:       RiskSafe,
			Why:        "ops_lint_fix runs golangci-lint with --fix to auto-correct mechanical lint violations.",
		},
		CodeGoTimeout: {
			Tool:       "ops_test_smart",
			Idempotent: true,
			Risk:       RiskSafe,
			Why:        "ops_test_smart narrows the test set to changed packages, avoiding the full-suite timeout.",
		},
		CodeIronbarStale: {
			Tool:       "systemd_restart",
			Args:       map[string]any{"unit": "dotfiles-ironbar.service", "scope": "user"},
			Idempotent: true,
			Risk:       RiskReload,
			Why:        "Ironbar has stopped rendering; a user-scope restart recovers without a session reload.",
		},
		CodeTickerStale: {
			Tool:       "keybinds_refresh_ticker",
			Idempotent: true,
			Risk:       RiskReload,
			Why:        "Keybind ticker consumer is showing a stale frame; refresh restarts the consumer service.",
		},
		CodeSwayncDrift: {
			Tool:       "dotfiles_reload_service",
			Args:       map[string]any{"service": "swaync"},
			Idempotent: true,
			Risk:       RiskReload,
			Why:        "Reloads swaync so recent config edits take effect without losing pinned state.",
		},
		CodeMcpkitVersionDrift: {
			Tool:       "dotfiles_mcpkit_version_sync",
			Args:       map[string]any{"execute": true},
			Idempotent: true,
			Risk:       RiskSafe,
			Why:        "Propagates the canonical mcpkit version across all downstream repos via 'go get mcpkit@latest' + 'go mod tidy'.",
		},
		CodeSystemdUnitFailed: {
			Tool:       "systemd_restart_verify",
			Idempotent: true,
			Risk:       RiskReload,
			Why:        "Clears the failed state and verifies the unit comes back up with fresh logs attached.",
		},
		CodePalettePropagationStale: {
			Tool:       "color_pipeline_apply",
			Idempotent: true,
			Risk:       RiskReload,
			Why:        "Re-renders matugen templates so every consumer reflects the current THEME_* tokens.",
		},
		CodeKittySocketLost: {
			Tool:       "systemd_restart",
			Args:       map[string]any{"unit": "kitty.service", "scope": "user"},
			Idempotent: false,
			Risk:       RiskDestructive,
			Why:        "Kitty remote-control socket is unavailable; a restart rebinds but will drop open terminal sessions.",
		},
		CodeMcpServerDead: {
			Tool:       "notify_send",
			Args: map[string]any{
				"title":   "MCP server is not responding",
				"body":    "Restart Claude Code or run scripts/run-dotfiles-mcp.sh manually to see the crash.",
				"urgency": "critical",
			},
			Idempotent: true,
			Risk:       RiskSafe,
			Why:        "Surfaces the dead-MCP condition to the user — no safe automated restart exists while Claude Code owns the process.",
		},
		CodeAudioSinkLost: {
			Tool:       "audio_device_switch",
			Idempotent: false,
			Risk:       RiskSafe,
			Why:        "Switch to the previously-active sink before it vanished; args should be filled with the prior sink name.",
		},
	}
}

// Lookup returns the remediation for a known error code. The returned
// Remediation is a copy — callers may mutate Args without affecting the
// registry.
func Lookup(code ErrorCode) (Remediation, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	r, ok := registry[code]
	if !ok {
		return Remediation{}, false
	}
	// Deep-copy Args so callers can mutate without tainting the registry.
	return cloneRemediation(r), true
}

// List returns the full registered catalog, sorted by code for stable output.
func List() []Entry {
	registryMu.RLock()
	defer registryMu.RUnlock()

	codes := make([]ErrorCode, 0, len(registry))
	for c := range registry {
		codes = append(codes, c)
	}
	sort.Slice(codes, func(i, j int) bool { return codes[i] < codes[j] })

	out := make([]Entry, 0, len(codes))
	for _, c := range codes {
		out = append(out, Entry{Code: c, Remediation: cloneRemediation(registry[c])})
	}
	return out
}

// Entry pairs a code with its remediation for catalog output.
type Entry struct {
	Code        ErrorCode   `json:"code"`
	Remediation Remediation `json:"remediation"`
}

func cloneRemediation(r Remediation) Remediation {
	var args map[string]any
	if len(r.Args) > 0 {
		args = make(map[string]any, len(r.Args))
		for k, v := range r.Args {
			args[k] = v
		}
	}
	return Remediation{
		Tool:       r.Tool,
		Args:       args,
		Idempotent: r.Idempotent,
		Risk:       r.Risk,
		Why:        r.Why,
	}
}
