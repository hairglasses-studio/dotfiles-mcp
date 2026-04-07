// mod_mapping_daemon.go — MCP tool for controlling the live mapitall daemon via IPC.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"

	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
)

// ===========================================================================
// I/O types
// ===========================================================================

type DaemonControlInput struct {
	Action   string `json:"action" jsonschema:"required,description=Action to perform,enum=status,enum=reload,enum=list_devices,enum=list_profiles,enum=get_state,enum=set_variable"`
	Variable string `json:"variable,omitempty" jsonschema:"description=Variable name (for set_variable action)"`
	Value    any    `json:"value,omitempty" jsonschema:"description=Variable value (for set_variable action)"`
}

type DaemonControlOutput struct {
	Action  string `json:"action"`
	Running bool   `json:"running"`
	Result  any    `json:"result,omitempty"`
	Error   string `json:"error,omitempty"`
}

// ===========================================================================
// Module
// ===========================================================================

type MappingDaemonModule struct{}

func (m *MappingDaemonModule) Name() string { return "mapping_daemon" }
func (m *MappingDaemonModule) Description() string {
	return "Control the live mapitall daemon via IPC"
}

func (m *MappingDaemonModule) Tools() []registry.ToolDefinition {
	return []registry.ToolDefinition{
		handler.TypedHandler[DaemonControlInput, DaemonControlOutput](
			"mapping_daemon_control",
			"Control the mapitall daemon. Actions: status (health/uptime), reload (hot-reload profiles), list_devices (connected controllers), list_profiles (loaded profiles), get_state (variables/active app), set_variable (set a runtime variable).",
			func(_ context.Context, input DaemonControlInput) (DaemonControlOutput, error) {
				socketPath := mapitallSocketPath()

				var method string
				var params any
				switch input.Action {
				case "status":
					method = "status"
				case "reload":
					method = "reload"
				case "list_devices":
					method = "list_devices"
				case "list_profiles":
					method = "list_profiles"
				case "get_state":
					method = "get_state"
				case "set_variable":
					if input.Variable == "" {
						return DaemonControlOutput{}, fmt.Errorf("[%s] variable name required for set_variable", handler.ErrInvalidParam)
					}
					method = "set_variable"
					params = map[string]any{"name": input.Variable, "value": input.Value}
				default:
					return DaemonControlOutput{}, fmt.Errorf("[%s] unknown action: %s", handler.ErrInvalidParam, input.Action)
				}

				result, err := mapitallCall(socketPath, method, params)
				if err != nil {
					return DaemonControlOutput{
						Action:  input.Action,
						Running: false,
						Error:   err.Error(),
					}, nil
				}

				return DaemonControlOutput{
					Action:  input.Action,
					Running: true,
					Result:  result,
				}, nil
			},
		),
	}
}

// ===========================================================================
// Self-contained JSON-RPC 2.0 client (no mapitall dependency)
// ===========================================================================

type jsonRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
	ID      int    `json:"id"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
	ID      int             `json:"id"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func mapitallCall(socketPath, method string, params any) (any, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w (is mapitall running?)", socketPath, err)
	}
	defer conn.Close()

	req := jsonRPCRequest{JSONRPC: "2.0", Method: method, Params: params, ID: 1}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return nil, err
	}

	var resp jsonRPCResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
	}

	var result any
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// mapitallSocketPath mirrors mapitall/internal/daemon/config.go:defaultSocketPath().
func mapitallSocketPath() string {
	if p := os.Getenv("MAPITALL_SOCKET"); p != "" {
		return p
	}
	switch runtime.GOOS {
	case "linux":
		if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
			return filepath.Join(dir, "mapitall.sock")
		}
		return "/tmp/mapitall.sock"
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Caches", "mapitall", "mapitall.sock")
	default:
		return "/tmp/mapitall.sock"
	}
}
