// mod_network.go — NetworkManager WiFi/VPN/DNS tools via nmcli + resolvectl
package dotfiles

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// netCheckTool checks if a CLI tool is available on PATH.
func netCheckTool(name string) error {
	_, err := exec.LookPath(name)
	if err != nil {
		return fmt.Errorf("%s not found on PATH — install it first (e.g. pacman -S %s)", name, name)
	}
	return nil
}

// netRunCmd executes a command with a context timeout and returns combined output.
func netRunCmd(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s failed: %w: %s", name, err, string(out))
	}
	return string(out), nil
}

// netRunCmdTimeout is a convenience wrapper that creates a context with the given timeout.
func netRunCmdTimeout(timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return netRunCmd(ctx, name, args...)
}

// parseTerseFields splits a nmcli --terse line by colon, handling escaped colons (\:).
func parseTerseFields(line string, count int) []string {
	fields := make([]string, 0, count)
	var current strings.Builder
	escaped := false
	for _, r := range line {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if r == ':' && len(fields) < count-1 {
			fields = append(fields, current.String())
			current.Reset()
			continue
		}
		current.WriteRune(r)
	}
	fields = append(fields, current.String())
	return fields
}

// ---------------------------------------------------------------------------
// Input types
// ---------------------------------------------------------------------------

// NetworkStatusInput defines the input for the network_status tool.
type NetworkStatusInput struct{}

// NetworkStatusOutput is the structured output for network_status.
type NetworkStatusOutput struct {
	State             string                    `json:"state"`
	Connectivity      string                    `json:"connectivity"`
	WifiEnabled       string                    `json:"wifi_enabled"`
	WwanEnabled       string                    `json:"wwan_enabled"`
	ActiveConnections []NetworkActiveConnection `json:"active_connections"`
}

// NetworkActiveConnection represents an active network connection.
type NetworkActiveConnection struct {
	Name   string `json:"name"`
	UUID   string `json:"uuid"`
	Type   string `json:"type"`
	Device string `json:"device"`
}

// NetworkWifiListInput defines the input for the network_wifi_list tool.
type NetworkWifiListInput struct {
	Rescan bool `json:"rescan,omitempty" jsonschema:"description=Force a WiFi rescan before listing (slower but more accurate)"`
}

// NetworkWifiEntry represents a single WiFi network.
type NetworkWifiEntry struct {
	SSID     string `json:"ssid"`
	Signal   int    `json:"signal"`
	Security string `json:"security"`
	Channel  string `json:"channel"`
	Freq     string `json:"freq"`
}

// NetworkWifiConnectInput defines the input for the network_wifi_connect tool.
type NetworkWifiConnectInput struct {
	SSID     string `json:"ssid" jsonschema:"required,description=WiFi network SSID to connect to"`
	Password string `json:"password,omitempty" jsonschema:"description=WiFi password (omit for open networks)"`
}

// NetworkVpnToggleInput defines the input for the network_vpn_toggle tool.
type NetworkVpnToggleInput struct {
	Name   string `json:"name" jsonschema:"required,description=Connection profile name"`
	Action string `json:"action" jsonschema:"required,description=Action to perform,enum=up,enum=down"`
}

// NetworkConnectionsInput defines the input for the network_connections tool.
type NetworkConnectionsInput struct{}

// NetworkConnectionEntry represents a saved connection profile.
type NetworkConnectionEntry struct {
	Name   string `json:"name"`
	UUID   string `json:"uuid"`
	Type   string `json:"type"`
	Device string `json:"device"`
}

// NetworkDNSInput defines the input for the network_dns tool.
type NetworkDNSInput struct{}

// NetworkDNSEntry represents DNS servers for an interface.
type NetworkDNSEntry struct {
	Interface string   `json:"interface"`
	Servers   []string `json:"servers"`
}

// NetworkWifiListOutput wraps a wifi network list in an object so MCP
// callers see a JSON object instead of a bare array (easier to extend
// with metadata such as the active SSID).
type NetworkWifiListOutput struct {
	Networks []NetworkWifiEntry `json:"networks"`
	Total    int                `json:"total"`
}

type NetworkConnectionsOutput struct {
	Connections []NetworkConnectionEntry `json:"connections"`
	Total       int                      `json:"total"`
}

type NetworkDNSOutput struct {
	Interfaces []NetworkDNSEntry `json:"interfaces"`
	Total      int               `json:"total"`
}

// ---------------------------------------------------------------------------
// Module
// ---------------------------------------------------------------------------

// NetworkModule provides NetworkManager WiFi/VPN/DNS control tools via nmcli.
type NetworkModule struct{}

func (m *NetworkModule) Name() string        { return "network" }
func (m *NetworkModule) Description() string { return "NetworkManager WiFi/VPN/DNS control via nmcli" }

func (m *NetworkModule) Tools() []registry.ToolDefinition {
	// ── network_status ───────────────────────────────────
	networkStatus := handler.TypedHandler[NetworkStatusInput, NetworkStatusOutput](
		"network_status",
		"Show overall network status: connectivity state, WiFi enabled, and active connections. Uses nmcli.",
		func(_ context.Context, _ NetworkStatusInput) (NetworkStatusOutput, error) {
			if err := netCheckTool("nmcli"); err != nil {
				return NetworkStatusOutput{}, err
			}

			// General status
			raw, err := netRunCmdTimeout(10*time.Second, "nmcli", "--terse", "general", "status")
			if err != nil {
				return NetworkStatusOutput{}, fmt.Errorf("nmcli general status: %w", err)
			}
			line := strings.TrimSpace(raw)
			fields := parseTerseFields(line, 7)

			out := NetworkStatusOutput{}
			if len(fields) >= 1 {
				out.State = fields[0]
			}
			if len(fields) >= 2 {
				out.Connectivity = fields[1]
			}
			if len(fields) >= 3 {
				out.WifiEnabled = fields[2]
			}
			if len(fields) >= 4 {
				out.WwanEnabled = fields[3]
			}

			// Active connections
			activeRaw, err := netRunCmdTimeout(10*time.Second, "nmcli", "--terse", "connection", "show", "--active")
			if err != nil {
				// Non-fatal: just no active connections
				return out, nil
			}

			for _, l := range strings.Split(strings.TrimSpace(activeRaw), "\n") {
				l = strings.TrimSpace(l)
				if l == "" {
					continue
				}
				f := parseTerseFields(l, 4)
				if len(f) >= 4 {
					out.ActiveConnections = append(out.ActiveConnections, NetworkActiveConnection{
						Name:   f[0],
						UUID:   f[1],
						Type:   f[2],
						Device: f[3],
					})
				}
			}

			return out, nil
		},
	)

	// ── network_wifi_list ────────────────────────────────
	networkWifiList := handler.TypedHandler[NetworkWifiListInput, NetworkWifiListOutput](
		"network_wifi_list",
		"Scan and list available WiFi networks sorted by signal strength. Returns SSID, signal, security, channel, and frequency. Deduplicates SSIDs keeping strongest signal.",
		func(_ context.Context, input NetworkWifiListInput) (NetworkWifiListOutput, error) {
			if err := netCheckTool("nmcli"); err != nil {
				return NetworkWifiListOutput{}, err
			}

			args := []string{"--terse", "--fields", "SSID,SIGNAL,SECURITY,CHAN,FREQ", "device", "wifi", "list"}
			if input.Rescan {
				args = append(args, "--rescan", "yes")
			}

			raw, err := netRunCmdTimeout(10*time.Second, "nmcli", args...)
			if err != nil {
				if strings.Contains(err.Error(), "NetworkManager is not running") {
					return NetworkWifiListOutput{}, fmt.Errorf("NetworkManager is not running — start it with: sudo systemctl start NetworkManager")
				}
				if strings.Contains(err.Error(), "Wi-Fi is disabled") || strings.Contains(err.Error(), "wifi is disabled") {
					return NetworkWifiListOutput{}, fmt.Errorf("WiFi is disabled — enable with: nmcli radio wifi on")
				}
				return NetworkWifiListOutput{}, fmt.Errorf("nmcli wifi list: %w", err)
			}

			// Parse and deduplicate by SSID (keep highest signal)
			seen := make(map[string]NetworkWifiEntry)
			for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				fields := parseTerseFields(line, 5)
				if len(fields) < 5 {
					continue
				}

				ssid := fields[0]
				if ssid == "" {
					continue
				}

				signal, _ := strconv.Atoi(fields[1])
				entry := NetworkWifiEntry{
					SSID:     ssid,
					Signal:   signal,
					Security: fields[2],
					Channel:  fields[3],
					Freq:     fields[4],
				}

				if existing, ok := seen[ssid]; !ok || signal > existing.Signal {
					seen[ssid] = entry
				}
			}

			// Collect and sort by signal descending
			result := make([]NetworkWifiEntry, 0, len(seen))
			for _, entry := range seen {
				result = append(result, entry)
			}
			sort.Slice(result, func(i, j int) bool {
				return result[i].Signal > result[j].Signal
			})

			return NetworkWifiListOutput{Networks: result, Total: len(result)}, nil
		},
	)

	// ── network_wifi_connect ─────────────────────────────
	networkWifiConnect := handler.TypedHandler[NetworkWifiConnectInput, string](
		"network_wifi_connect",
		"DESTRUCTIVE: Connect to a WiFi network by SSID. Provide password for protected networks, omit for open networks. Requires explicit approval.",
		func(_ context.Context, input NetworkWifiConnectInput) (string, error) {
			if err := netCheckTool("nmcli"); err != nil {
				return "", err
			}

			if input.SSID == "" {
				return "", fmt.Errorf("[%s] ssid must not be empty", handler.ErrInvalidParam)
			}

			args := []string{"device", "wifi", "connect", input.SSID}
			if input.Password != "" {
				args = append(args, "password", input.Password)
			}

			raw, err := netRunCmdTimeout(30*time.Second, "nmcli", args...)
			if err != nil {
				return "", fmt.Errorf("wifi connect failed: %w", err)
			}

			return strings.TrimSpace(raw), nil
		},
	)
	networkWifiConnect.IsWrite = true

	// ── network_vpn_toggle ───────────────────────────────
	networkVpnToggle := handler.TypedHandler[NetworkVpnToggleInput, string](
		"network_vpn_toggle",
		"DESTRUCTIVE: Connect or disconnect a VPN/connection profile by name. Use action 'up' to connect and 'down' to disconnect. Requires explicit approval.",
		func(_ context.Context, input NetworkVpnToggleInput) (string, error) {
			if err := netCheckTool("nmcli"); err != nil {
				return "", err
			}

			if input.Name == "" {
				return "", fmt.Errorf("[%s] name must not be empty", handler.ErrInvalidParam)
			}

			switch input.Action {
			case "up":
				raw, err := netRunCmdTimeout(30*time.Second, "nmcli", "connection", "up", input.Name)
				if err != nil {
					return "", fmt.Errorf("connection up failed: %w", err)
				}
				return strings.TrimSpace(raw), nil
			case "down":
				raw, err := netRunCmdTimeout(10*time.Second, "nmcli", "connection", "down", input.Name)
				if err != nil {
					return "", fmt.Errorf("connection down failed: %w", err)
				}
				return strings.TrimSpace(raw), nil
			default:
				return "", fmt.Errorf("[%s] action must be 'up' or 'down', got %q", handler.ErrInvalidParam, input.Action)
			}
		},
	)
	networkVpnToggle.IsWrite = true

	// ── network_connections ──────────────────────────────
	networkConnections := handler.TypedHandler[NetworkConnectionsInput, NetworkConnectionsOutput](
		"network_connections",
		"List all saved NetworkManager connection profiles (WiFi, VPN, Ethernet, etc.) with name, UUID, type, and device.",
		func(_ context.Context, _ NetworkConnectionsInput) (NetworkConnectionsOutput, error) {
			if err := netCheckTool("nmcli"); err != nil {
				return NetworkConnectionsOutput{}, err
			}

			raw, err := netRunCmdTimeout(10*time.Second, "nmcli", "--terse", "connection", "show")
			if err != nil {
				return NetworkConnectionsOutput{}, fmt.Errorf("nmcli connection show: %w", err)
			}

			var result []NetworkConnectionEntry
			for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				fields := parseTerseFields(line, 4)
				if len(fields) >= 4 {
					result = append(result, NetworkConnectionEntry{
						Name:   fields[0],
						UUID:   fields[1],
						Type:   fields[2],
						Device: fields[3],
					})
				}
			}

			return NetworkConnectionsOutput{Connections: result, Total: len(result)}, nil
		},
	)

	// ── network_dns ──────────────────────────────────────
	networkDNS := handler.TypedHandler[NetworkDNSInput, NetworkDNSOutput](
		"network_dns",
		"Show DNS servers per network interface. Tries resolvectl first, falls back to /etc/resolv.conf.",
		func(_ context.Context, _ NetworkDNSInput) (NetworkDNSOutput, error) {
			// Try resolvectl first; fall back to /etc/resolv.conf
			raw, err := netRunCmdTimeout(10*time.Second, "resolvectl", "status")
			if err != nil {
				// Fallback: parse /etc/resolv.conf
				data, readErr := os.ReadFile("/etc/resolv.conf")
				if readErr != nil {
					return NetworkDNSOutput{}, fmt.Errorf("resolvectl failed (%w) and /etc/resolv.conf unreadable: %v", err, readErr)
				}
				var servers []string
				for _, line := range strings.Split(string(data), "\n") {
					line = strings.TrimSpace(line)
					if strings.HasPrefix(line, "nameserver ") {
						servers = append(servers, strings.TrimSpace(strings.TrimPrefix(line, "nameserver ")))
					}
				}
				entry := NetworkDNSEntry{Interface: "resolv.conf", Servers: servers}
				return NetworkDNSOutput{Interfaces: []NetworkDNSEntry{entry}, Total: 1}, nil
			}
			if err != nil {
				return NetworkDNSOutput{}, fmt.Errorf("resolvectl status: %w", err)
			}

			var result []NetworkDNSEntry
			var current *NetworkDNSEntry

			for _, line := range strings.Split(raw, "\n") {
				trimmed := strings.TrimSpace(line)

				// Detect interface headers like "Link 2 (enp5s0)" or "Link 3 (wlp4s0)"
				if strings.HasPrefix(trimmed, "Link ") && strings.Contains(trimmed, "(") && strings.Contains(trimmed, ")") {
					if current != nil && len(current.Servers) > 0 {
						result = append(result, *current)
					}
					start := strings.Index(trimmed, "(")
					end := strings.Index(trimmed, ")")
					ifName := ""
					if start >= 0 && end > start {
						ifName = trimmed[start+1 : end]
					}
					current = &NetworkDNSEntry{Interface: ifName}
					continue
				}

				// Parse DNS Servers lines
				if current != nil {
					if strings.HasPrefix(trimmed, "DNS Servers:") {
						server := strings.TrimSpace(strings.TrimPrefix(trimmed, "DNS Servers:"))
						if server != "" {
							current.Servers = append(current.Servers, server)
						}
						continue
					}
					// Continuation lines for DNS servers (indented IPs after "DNS Servers:")
					if len(current.Servers) > 0 && trimmed != "" && !strings.Contains(trimmed, ":") {
						// Pure IP address continuation line
						current.Servers = append(current.Servers, trimmed)
						continue
					}
					// Additional DNS server line (indented, just an IP)
					if len(current.Servers) > 0 && isIPAddress(trimmed) {
						current.Servers = append(current.Servers, trimmed)
						continue
					}
				}
			}
			// Flush last entry
			if current != nil && len(current.Servers) > 0 {
				result = append(result, *current)
			}

			return NetworkDNSOutput{Interfaces: result, Total: len(result)}, nil
		},
	)

	// ── network_dns_set ──────────────────────────────
	networkDNSSet := handler.TypedHandler[NetworkDNSSetInput, NetworkDNSSetOutput](
		"network_dns_set",
		"Set DNS servers on a NetworkManager profile via `nmcli connection modify <profile> ipv4.dns`. Requires the connection to be brought back up to take effect — pass reapply=true (default) to do both steps. Passing an empty servers list clears the override so the profile falls back to the DHCP-provided DNS.",
		func(_ context.Context, input NetworkDNSSetInput) (NetworkDNSSetOutput, error) {
			if err := netCheckTool("nmcli"); err != nil {
				return NetworkDNSSetOutput{}, err
			}
			profile := strings.TrimSpace(input.Profile)
			if profile == "" {
				return NetworkDNSSetOutput{}, fmt.Errorf("[%s] profile is required", handler.ErrInvalidParam)
			}
			// Validate each server is an IP to prevent shell-quoted injection.
			for _, s := range input.Servers {
				s = strings.TrimSpace(s)
				if s == "" {
					continue
				}
				if !isIPAddress(s) {
					return NetworkDNSSetOutput{}, fmt.Errorf("[%s] %q is not a valid IP address", handler.ErrInvalidParam, s)
				}
			}
			joined := strings.Join(input.Servers, ",")
			_, err := netRunCmdTimeout(15*time.Second, "nmcli", "connection", "modify", profile, "ipv4.dns", joined)
			if err != nil {
				return NetworkDNSSetOutput{Profile: profile}, fmt.Errorf("nmcli connection modify %s ipv4.dns: %w", profile, err)
			}
			out := NetworkDNSSetOutput{Profile: profile, Servers: input.Servers, Applied: true}
			reapply := true
			if input.Reapply != nil {
				reapply = *input.Reapply
			}
			if reapply {
				// `nmcli connection up` re-reads the modified settings and
				// restarts the connection — required for DNS changes to
				// propagate to resolvectl / resolv.conf.
				if _, err := netRunCmdTimeout(30*time.Second, "nmcli", "connection", "up", profile); err != nil {
					out.Reapplied = false
					out.Notes = fmt.Sprintf("DNS saved, but reapply failed: %v", err)
					return out, nil
				}
				out.Reapplied = true
			}
			return out, nil
		},
	)

	return []registry.ToolDefinition{
		networkStatus,
		networkWifiList,
		networkWifiConnect,
		networkVpnToggle,
		networkConnections,
		networkDNS,
		networkDNSSet,
	}
}

// NetworkDNSSetInput configures DNS on a NetworkManager profile.
type NetworkDNSSetInput struct {
	Profile string   `json:"profile" jsonschema:"required,description=NetworkManager connection profile name (see network_connections)"`
	Servers []string `json:"servers" jsonschema:"description=IPv4 addresses to set as DNS. Empty list clears the override and falls back to DHCP-provided DNS."`
	Reapply *bool    `json:"reapply,omitempty" jsonschema:"description=After modifying, call 'nmcli connection up' so DNS takes effect immediately (default: true)"`
}

type NetworkDNSSetOutput struct {
	Profile   string   `json:"profile"`
	Servers   []string `json:"servers,omitempty"`
	Applied   bool     `json:"applied"`
	Reapplied bool     `json:"reapplied,omitempty"`
	Notes     string   `json:"notes,omitempty"`
}

// isIPAddress checks if a string looks like an IP address (v4 or v6).
func isIPAddress(s string) bool {
	if s == "" {
		return false
	}
	// Simple heuristic: IPv4 has dots and digits, IPv6 has colons and hex
	if strings.Count(s, ".") == 3 {
		for _, c := range s {
			if c != '.' && (c < '0' || c > '9') {
				return false
			}
		}
		return true
	}
	if strings.Contains(s, ":") {
		for _, c := range s {
			if c != ':' && !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
		return true
	}
	return false
}
