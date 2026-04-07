// mod_system.go — System monitoring tools: temps, GPU, disk, memory, updates, uptime
package dotfiles

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
)

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const sysCmdTimeout = 10 * time.Second

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// sysRunCmd executes a command with a timeout and returns combined output.
func sysRunCmd(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), sysCmdTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s failed: %w: %s", name, err, string(out))
	}
	return string(out), nil
}

// sysCheckTool checks if a CLI tool is available on PATH.
func sysCheckTool(name string) error {
	_, err := exec.LookPath(name)
	if err != nil {
		return fmt.Errorf("%s not found on PATH", name)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Input types (all read-only, minimal params)
// ---------------------------------------------------------------------------

// SystemTempsInput is the input for system_temps (no params needed).
type SystemTempsInput struct{}

// SystemGPUInput is the input for system_gpu (no params needed).
type SystemGPUInput struct{}

// SystemDiskInput is the input for system_disk (no params needed).
type SystemDiskInput struct{}

// SystemUpdatesInput is the input for system_updates (no params needed).
type SystemUpdatesInput struct{}

// SystemMemoryInput is the input for system_memory (no params needed).
type SystemMemoryInput struct{}

// SystemUptimeInput is the input for system_uptime (no params needed).
type SystemUptimeInput struct{}

// ---------------------------------------------------------------------------
// Output types
// ---------------------------------------------------------------------------

// FanReading represents a single fan sensor reading.
type FanReading struct {
	Name string `json:"name"`
	RPM  int    `json:"rpm"`
}

// SystemTempsOutput is the structured response from system_temps.
type SystemTempsOutput struct {
	CPUTemp      float64      `json:"cpu_temp"`
	GPUTemp      float64      `json:"gpu_temp"`
	NVMeTemp     float64      `json:"nvme_temp"`
	Fans         []FanReading `json:"fans"`
	NotAvailable string       `json:"not_available,omitempty"`
}

// SystemGPUOutput is the structured response from system_gpu.
type SystemGPUOutput struct {
	Name         string  `json:"name"`
	Temp         int     `json:"temp"`
	GPUUtil      int     `json:"gpu_util"`
	MemUtil      int     `json:"mem_util"`
	MemUsedMB    int     `json:"mem_used_mb"`
	MemTotalMB   int     `json:"mem_total_mb"`
	PowerW       float64 `json:"power_w"`
	NotAvailable string  `json:"not_available,omitempty"`
}

// DiskMount represents a single filesystem mount's usage.
type DiskMount struct {
	Mount   string `json:"mount"`
	Total   string `json:"total"`
	Used    string `json:"used"`
	Avail   string `json:"avail"`
	Percent string `json:"percent"`
}

// SystemDiskOutput is the structured response from system_disk.
type SystemDiskOutput struct {
	Mounts []DiskMount `json:"mounts"`
}

// SystemUpdatesOutput is the structured response from system_updates.
type SystemUpdatesOutput struct {
	PacmanCount  int    `json:"pacman_count"`
	AURCount     int    `json:"aur_count"`
	Total        int    `json:"total"`
	NotAvailable string `json:"not_available,omitempty"`
}

// SystemMemoryOutput is the structured response from system_memory.
type SystemMemoryOutput struct {
	TotalMB     int     `json:"total_mb"`
	UsedMB      int     `json:"used_mb"`
	AvailableMB int     `json:"available_mb"`
	Percent     float64 `json:"percent"`
	SwapTotalMB int     `json:"swap_total_mb"`
	SwapUsedMB  int     `json:"swap_used_mb"`
}

// SystemUptimeOutput is the structured response from system_uptime.
type SystemUptimeOutput struct {
	UptimeHours float64 `json:"uptime_hours"`
	Load1m      float64 `json:"load_1m"`
	Load5m      float64 `json:"load_5m"`
	Load15m     float64 `json:"load_15m"`
	LastBoot    string  `json:"last_boot"`
}

// ---------------------------------------------------------------------------
// Tool implementations
// ---------------------------------------------------------------------------

func systemTemps(_ context.Context, _ SystemTempsInput) (SystemTempsOutput, error) {
	out := SystemTempsOutput{}

	if err := sysCheckTool("sensors"); err != nil {
		out.NotAvailable = "sensors (lm_sensors) not installed"
		return out, nil
	}

	raw, err := sysRunCmd("sensors", "-j")
	if err != nil {
		out.NotAvailable = fmt.Sprintf("sensors command failed: %v", err)
		return out, nil
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		out.NotAvailable = fmt.Sprintf("failed to parse sensors JSON: %v", err)
		return out, nil
	}

	// Walk the nested JSON to find known sensor values.
	// Structure: { "chip-name": { "sensor-label": { "temp1_input": float } } }
	for chipName, chipVal := range data {
		chip, ok := chipVal.(map[string]any)
		if !ok {
			continue
		}
		for sensorName, sensorVal := range chip {
			sensor, ok := sensorVal.(map[string]any)
			if !ok {
				continue
			}

			// CPU temp: k10temp (AMD) or coretemp (Intel)
			if strings.Contains(chipName, "k10temp") || strings.Contains(chipName, "coretemp") {
				if sensorName == "Tctl" || sensorName == "Tdie" || strings.HasPrefix(sensorName, "Core 0") {
					if v, ok := sensorFloat(sensor, "temp1_input"); ok {
						out.CPUTemp = v
					}
				}
			}

			// GPU temp: amdgpu
			if strings.Contains(chipName, "amdgpu") && sensorName == "edge" {
				if v, ok := sensorFloat(sensor, "temp1_input"); ok {
					out.GPUTemp = v
				}
			}

			// NVMe temp
			if strings.Contains(chipName, "nvme") && sensorName == "Composite" {
				if v, ok := sensorFloat(sensor, "temp1_input"); ok {
					out.NVMeTemp = v
				}
			}

			// Fan readings
			if strings.Contains(sensorName, "fan") {
				for key, val := range sensor {
					if strings.HasSuffix(key, "_input") {
						if rpm, ok := toFloat(val); ok && rpm > 0 {
							out.Fans = append(out.Fans, FanReading{
								Name: sensorName,
								RPM:  int(rpm),
							})
						}
					}
				}
			}
		}
	}

	return out, nil
}

// sensorFloat extracts a float64 from a sensor sub-map by key.
func sensorFloat(sensor map[string]any, key string) (float64, bool) {
	v, ok := sensor[key]
	if !ok {
		return 0, false
	}
	return toFloat(v)
}

// toFloat converts an interface{} to float64.
func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

func systemGPU(_ context.Context, _ SystemGPUInput) (SystemGPUOutput, error) {
	out := SystemGPUOutput{}

	if err := sysCheckTool("nvidia-smi"); err != nil {
		out.NotAvailable = "nvidia-smi not found — no NVIDIA GPU or driver not installed"
		return out, nil
	}

	raw, err := sysRunCmd("nvidia-smi",
		"--query-gpu=name,temperature.gpu,utilization.gpu,utilization.memory,memory.used,memory.total,power.draw",
		"--format=csv,noheader,nounits",
	)
	if err != nil {
		out.NotAvailable = fmt.Sprintf("nvidia-smi failed: %v", err)
		return out, nil
	}

	// Parse CSV: "NVIDIA GeForce RTX 4090, 45, 12, 8, 1024, 24564, 85.50"
	fields := strings.Split(strings.TrimSpace(raw), ", ")
	if len(fields) < 7 {
		// Try comma-only split (some versions don't pad with space)
		fields = strings.Split(strings.TrimSpace(raw), ",")
		for i := range fields {
			fields[i] = strings.TrimSpace(fields[i])
		}
	}
	if len(fields) < 7 {
		out.NotAvailable = fmt.Sprintf("unexpected nvidia-smi output format: %s", raw)
		return out, nil
	}

	out.Name = fields[0]
	out.Temp, _ = strconv.Atoi(fields[1])
	out.GPUUtil, _ = strconv.Atoi(fields[2])
	out.MemUtil, _ = strconv.Atoi(fields[3])
	out.MemUsedMB, _ = strconv.Atoi(fields[4])
	out.MemTotalMB, _ = strconv.Atoi(fields[5])
	out.PowerW, _ = strconv.ParseFloat(fields[6], 64)

	return out, nil
}

func systemDisk(_ context.Context, _ SystemDiskInput) (SystemDiskOutput, error) {
	raw, err := sysRunCmd("df", "--output=target,size,used,avail,pcent", "-x", "tmpfs", "-x", "devtmpfs")
	if err != nil {
		return SystemDiskOutput{}, fmt.Errorf("[%s] df command failed: %v", handler.ErrInvalidParam, err)
	}

	var mounts []DiskMount
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	for i, line := range lines {
		if i == 0 {
			continue // skip header
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		mounts = append(mounts, DiskMount{
			Mount:   fields[0],
			Total:   fields[1],
			Used:    fields[2],
			Avail:   fields[3],
			Percent: fields[4],
		})
	}

	return SystemDiskOutput{Mounts: mounts}, nil
}

func systemUpdates(_ context.Context, _ SystemUpdatesInput) (SystemUpdatesOutput, error) {
	out := SystemUpdatesOutput{}

	// checkupdates (pacman-contrib)
	if err := sysCheckTool("checkupdates"); err == nil {
		raw, err := sysRunCmd("checkupdates")
		if err == nil {
			out.PacmanCount = countNonEmptyLines(raw)
		}
		// checkupdates exits 2 when no updates, which is not an error for us
	}

	// yay -Qu (AUR updates)
	if err := sysCheckTool("yay"); err == nil {
		raw, err := sysRunCmd("yay", "-Qua")
		if err == nil {
			out.AURCount = countNonEmptyLines(raw)
		}
	} else {
		out.NotAvailable = "yay not installed — AUR count unavailable"
	}

	out.Total = out.PacmanCount + out.AURCount
	return out, nil
}

// countNonEmptyLines counts lines with actual content.
func countNonEmptyLines(s string) int {
	count := 0
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

func systemMemory(_ context.Context, _ SystemMemoryInput) (SystemMemoryOutput, error) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return SystemMemoryOutput{}, fmt.Errorf("[%s] failed to read /proc/meminfo: %v", handler.ErrInvalidParam, err)
	}

	values := parseProcMeminfo(string(data))

	totalKB := values["MemTotal"]
	availKB := values["MemAvailable"]
	swapTotalKB := values["SwapTotal"]
	swapFreeKB := values["SwapFree"]

	totalMB := totalKB / 1024
	availMB := availKB / 1024
	usedMB := totalMB - availMB
	swapTotalMB := swapTotalKB / 1024
	swapUsedMB := (swapTotalKB - swapFreeKB) / 1024

	var pct float64
	if totalMB > 0 {
		pct = float64(usedMB) / float64(totalMB) * 100
		// Round to one decimal
		pct = float64(int(pct*10)) / 10
	}

	return SystemMemoryOutput{
		TotalMB:     totalMB,
		UsedMB:      usedMB,
		AvailableMB: availMB,
		Percent:     pct,
		SwapTotalMB: swapTotalMB,
		SwapUsedMB:  swapUsedMB,
	}, nil
}

// parseProcMeminfo parses /proc/meminfo into a map of key -> value in kB.
func parseProcMeminfo(content string) map[string]int {
	result := make(map[string]int)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Format: "MemTotal:       32768000 kB"
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		valStr := strings.TrimSpace(parts[1])
		valStr = strings.TrimSuffix(valStr, " kB")
		valStr = strings.TrimSpace(valStr)
		val, err := strconv.Atoi(valStr)
		if err != nil {
			continue
		}
		result[key] = val
	}
	return result
}

func systemUptime(_ context.Context, _ SystemUptimeInput) (SystemUptimeOutput, error) {
	out := SystemUptimeOutput{}

	// /proc/uptime: "seconds_up idle_seconds"
	uptimeData, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return out, fmt.Errorf("[%s] failed to read /proc/uptime: %v", handler.ErrInvalidParam, err)
	}
	fields := strings.Fields(string(uptimeData))
	if len(fields) >= 1 {
		secs, err := strconv.ParseFloat(fields[0], 64)
		if err == nil {
			out.UptimeHours = float64(int(secs/3600*10)) / 10 // one decimal
		}
	}

	// /proc/loadavg: "1min 5min 15min running/total last_pid"
	loadData, err := os.ReadFile("/proc/loadavg")
	if err == nil {
		fields := strings.Fields(string(loadData))
		if len(fields) >= 3 {
			out.Load1m, _ = strconv.ParseFloat(fields[0], 64)
			out.Load5m, _ = strconv.ParseFloat(fields[1], 64)
			out.Load15m, _ = strconv.ParseFloat(fields[2], 64)
		}
	}

	// who -b: "         system boot  2025-04-01 10:30"
	bootRaw, err := sysRunCmd("who", "-b")
	if err == nil {
		// Extract the timestamp after "system boot"
		if idx := strings.Index(bootRaw, "system boot"); idx >= 0 {
			out.LastBoot = strings.TrimSpace(bootRaw[idx+len("system boot"):])
		}
	}

	return out, nil
}

// ---------------------------------------------------------------------------
// Module
// ---------------------------------------------------------------------------

// SystemModule provides system monitoring tools.
type SystemModule struct{}

func (m *SystemModule) Name() string { return "system" }
func (m *SystemModule) Description() string {
	return "System monitoring: temps, GPU, disk, memory, updates, uptime"
}

func (m *SystemModule) Tools() []registry.ToolDefinition {
	return []registry.ToolDefinition{
		handler.TypedHandler[SystemTempsInput, SystemTempsOutput](
			"system_temps",
			"Read CPU, GPU, and NVMe temperatures plus fan RPMs via lm_sensors. Returns structured sensor data. Gracefully returns partial data if sensors is not installed.",
			systemTemps,
		),
		handler.TypedHandler[SystemGPUInput, SystemGPUOutput](
			"system_gpu",
			"Read NVIDIA GPU stats: name, temperature, GPU/memory utilization, VRAM usage, and power draw via nvidia-smi. Returns not_available if no NVIDIA GPU is present.",
			systemGPU,
		),
		handler.TypedHandler[SystemDiskInput, SystemDiskOutput](
			"system_disk",
			"List disk usage per mount point (excluding tmpfs/devtmpfs). Returns total, used, available, and percent for each filesystem.",
			systemDisk,
		),
		handler.TypedHandler[SystemUpdatesInput, SystemUpdatesOutput](
			"system_updates",
			"Check pending package updates. Returns pacman (official) and AUR (via yay) update counts. Graceful if checkupdates or yay is not installed.",
			systemUpdates,
		),
		handler.TypedHandler[SystemMemoryInput, SystemMemoryOutput](
			"system_memory",
			"Read RAM and swap usage from /proc/meminfo. Returns total, used, available in MB with usage percentage.",
			systemMemory,
		),
		handler.TypedHandler[SystemUptimeInput, SystemUptimeOutput](
			"system_uptime",
			"Read system uptime, load averages (1/5/15 min), and last boot time. Uses /proc/uptime, /proc/loadavg, and who -b.",
			systemUptime,
		),
		handler.TypedHandler[SystemHealthCheckInput, SystemHealthCheckOutput](
			"system_health_check",
			"Composed health dashboard: aggregates temps, GPU, memory, disk, uptime, and updates into a single report with threshold-based warnings. Returns overall OK/WARN/CRIT status with per-subsystem detail.",
			systemHealthCheck,
		),
	}
}

// ---------------------------------------------------------------------------
// Composed tool: system_health_check
// ---------------------------------------------------------------------------

// SystemHealthCheckInput configures warning thresholds for the health check.
type SystemHealthCheckInput struct {
	WarnCPUTemp   float64 `json:"warn_cpu_temp,omitempty" jsonschema:"description=CPU temp warning threshold in Celsius (default 85)"`
	WarnDiskPct   int     `json:"warn_disk_pct,omitempty" jsonschema:"description=Disk usage warning threshold percentage (default 90)"`
	WarnMemoryPct float64 `json:"warn_memory_pct,omitempty" jsonschema:"description=Memory usage warning threshold percentage (default 85)"`
}

// SystemHealthCheckOutput is the structured health report.
type SystemHealthCheckOutput struct {
	Overall    string                  `json:"overall"`
	Subsystems []SystemHealthSubsystem `json:"subsystems"`
	Warnings   []string                `json:"warnings"`
	Summary    string                  `json:"summary"`
}

// SystemHealthSubsystem represents one subsystem's health status.
type SystemHealthSubsystem struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Value  string `json:"value"`
	Detail any    `json:"detail"`
}

func systemHealthCheck(ctx context.Context, input SystemHealthCheckInput) (SystemHealthCheckOutput, error) {
	// Apply defaults.
	warnCPU := input.WarnCPUTemp
	if warnCPU <= 0 {
		warnCPU = 85
	}
	warnDisk := input.WarnDiskPct
	if warnDisk <= 0 {
		warnDisk = 90
	}
	warnMem := input.WarnMemoryPct
	if warnMem <= 0 {
		warnMem = 85
	}

	out := SystemHealthCheckOutput{
		Overall: "OK",
	}

	// 1. CPU/GPU/NVMe temps
	temps, _ := systemTemps(ctx, SystemTempsInput{})
	if temps.NotAvailable == "" {
		status := "ok"
		if temps.CPUTemp > 95 {
			status = "crit"
			out.Warnings = append(out.Warnings, fmt.Sprintf("CPU temp CRITICAL: %.0f°C > 95°C", temps.CPUTemp))
		} else if temps.CPUTemp > warnCPU {
			status = "warn"
			out.Warnings = append(out.Warnings, fmt.Sprintf("CPU temp high: %.0f°C > %.0f°C threshold", temps.CPUTemp, warnCPU))
		}
		out.Subsystems = append(out.Subsystems, SystemHealthSubsystem{
			Name:   "cpu_temp",
			Status: status,
			Value:  fmt.Sprintf("%.0f°C", temps.CPUTemp),
			Detail: temps,
		})
	} else {
		out.Subsystems = append(out.Subsystems, SystemHealthSubsystem{
			Name:   "cpu_temp",
			Status: "ok",
			Value:  temps.NotAvailable,
			Detail: nil,
		})
	}

	// 2. GPU stats
	gpu, _ := systemGPU(ctx, SystemGPUInput{})
	if gpu.NotAvailable == "" {
		status := "ok"
		gpuTemp := float64(gpu.Temp)
		if gpuTemp > 100 {
			status = "crit"
			out.Warnings = append(out.Warnings, fmt.Sprintf("GPU temp CRITICAL: %d°C > 100°C", gpu.Temp))
		} else if gpuTemp > 90 {
			status = "warn"
			out.Warnings = append(out.Warnings, fmt.Sprintf("GPU temp high: %d°C > 90°C", gpu.Temp))
		}
		out.Subsystems = append(out.Subsystems, SystemHealthSubsystem{
			Name:   "gpu",
			Status: status,
			Value:  fmt.Sprintf("%d°C, %d%% util, %.0fW", gpu.Temp, gpu.GPUUtil, gpu.PowerW),
			Detail: gpu,
		})
	} else {
		out.Subsystems = append(out.Subsystems, SystemHealthSubsystem{
			Name:   "gpu",
			Status: "ok",
			Value:  gpu.NotAvailable,
			Detail: nil,
		})
	}

	// 3. Memory
	mem, err := systemMemory(ctx, SystemMemoryInput{})
	if err == nil {
		status := "ok"
		if mem.Percent > 95 {
			status = "crit"
			out.Warnings = append(out.Warnings, fmt.Sprintf("Memory CRITICAL: %.1f%% > 95%%", mem.Percent))
		} else if mem.Percent > warnMem {
			status = "warn"
			out.Warnings = append(out.Warnings, fmt.Sprintf("Memory high: %.1f%% > %.0f%% threshold", mem.Percent, warnMem))
		}
		out.Subsystems = append(out.Subsystems, SystemHealthSubsystem{
			Name:   "memory",
			Status: status,
			Value:  fmt.Sprintf("%.1f%% (%dMB/%dMB)", mem.Percent, mem.UsedMB, mem.TotalMB),
			Detail: mem,
		})
	}

	// 4. Disk
	disk, err := systemDisk(ctx, SystemDiskInput{})
	if err == nil {
		worstStatus := "ok"
		for _, m := range disk.Mounts {
			pctStr := strings.TrimSuffix(m.Percent, "%")
			pct, _ := strconv.Atoi(pctStr)
			if pct > 98 {
				worstStatus = "crit"
				out.Warnings = append(out.Warnings, fmt.Sprintf("Disk CRITICAL: %s at %s", m.Mount, m.Percent))
			} else if pct > warnDisk && worstStatus != "crit" {
				worstStatus = "warn"
				out.Warnings = append(out.Warnings, fmt.Sprintf("Disk high: %s at %s > %d%% threshold", m.Mount, m.Percent, warnDisk))
			}
		}
		// Summary value: show the highest-usage mount.
		highMount, highPct := "", 0
		for _, m := range disk.Mounts {
			pctStr := strings.TrimSuffix(m.Percent, "%")
			pct, _ := strconv.Atoi(pctStr)
			if pct > highPct {
				highPct = pct
				highMount = m.Mount
			}
		}
		out.Subsystems = append(out.Subsystems, SystemHealthSubsystem{
			Name:   "disk",
			Status: worstStatus,
			Value:  fmt.Sprintf("highest: %s at %d%%", highMount, highPct),
			Detail: disk,
		})
	}

	// 5. Uptime / load
	uptime, err := systemUptime(ctx, SystemUptimeInput{})
	if err == nil {
		status := "ok"
		cpuCores := float64(runtime.NumCPU())
		if uptime.Load1m > cpuCores*2 {
			status = "warn"
			out.Warnings = append(out.Warnings, fmt.Sprintf("Load average high: %.2f > %.0f (2x %d cores)", uptime.Load1m, cpuCores*2, int(cpuCores)))
		}
		out.Subsystems = append(out.Subsystems, SystemHealthSubsystem{
			Name:   "load",
			Status: status,
			Value:  fmt.Sprintf("%.2f / %.2f / %.2f (up %.1fh)", uptime.Load1m, uptime.Load5m, uptime.Load15m, uptime.UptimeHours),
			Detail: uptime,
		})
	}

	// 6. Updates
	updates, _ := systemUpdates(ctx, SystemUpdatesInput{})
	{
		status := "ok"
		if updates.Total > 50 {
			status = "warn"
			out.Warnings = append(out.Warnings, fmt.Sprintf("Pending updates: %d (>50)", updates.Total))
		}
		out.Subsystems = append(out.Subsystems, SystemHealthSubsystem{
			Name:   "updates",
			Status: status,
			Value:  fmt.Sprintf("%d pending (%d pacman, %d AUR)", updates.Total, updates.PacmanCount, updates.AURCount),
			Detail: updates,
		})
	}

	// Compute overall status.
	for _, sub := range out.Subsystems {
		if sub.Status == "crit" {
			out.Overall = "CRIT"
			break
		}
		if sub.Status == "warn" && out.Overall != "CRIT" {
			out.Overall = "WARN"
		}
	}

	// Build summary.
	okCount, warnCount, critCount := 0, 0, 0
	for _, sub := range out.Subsystems {
		switch sub.Status {
		case "ok":
			okCount++
		case "warn":
			warnCount++
		case "crit":
			critCount++
		}
	}
	out.Summary = fmt.Sprintf("%s — %d/%d subsystems OK, %d warnings, %d critical",
		out.Overall, okCount, len(out.Subsystems), warnCount, critCount)

	return out, nil
}
