// mod_process.go — Process management, port inspection, GPU status, and system info
package main

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// procRunCmd executes a command and returns stdout, stderr, and error separately.
func procRunCmd(ctx context.Context, name string, args ...string) (string, string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), err
}

// procReadProcFile reads and trims a file from /proc.
func procReadProcFile(name string) (string, error) {
	data, err := os.ReadFile("/proc/" + name)
	if err != nil {
		return "", fmt.Errorf("read /proc/%s: %w", name, err)
	}
	return strings.TrimSpace(string(data)), nil
}

// ---------------------------------------------------------------------------
// Input / output types
// ---------------------------------------------------------------------------

type ProcPsListInput struct {
	Filter string `json:"filter,omitempty" jsonschema:"description=Filter processes by command substring"`
	SortBy string `json:"sort_by,omitempty" jsonschema:"description=Sort by: cpu (default)\\, mem\\, or pid"`
	Limit  int    `json:"limit,omitempty" jsonschema:"description=Max processes to return. Default 20."`
}

type ProcProcessInfo struct {
	User    string  `json:"user"`
	PID     int     `json:"pid"`
	CPU     float64 `json:"cpu"`
	Mem     float64 `json:"mem"`
	VSZ     int     `json:"vsz"`
	RSS     int     `json:"rss"`
	TTY     string  `json:"tty"`
	Stat    string  `json:"stat"`
	Start   string  `json:"start"`
	Time    string  `json:"time"`
	Command string  `json:"command"`
}

type ProcPsListOutput struct {
	Processes []ProcProcessInfo `json:"processes"`
	Total     int               `json:"total"`
}

type ProcPsTreeInput struct {
	PID int `json:"pid" jsonschema:"description=Process ID to show tree for,required"`
}

type ProcPsTreeOutput struct {
	PID  int    `json:"pid"`
	Tree string `json:"tree"`
}

type ProcKillInput struct {
	PID    int    `json:"pid" jsonschema:"description=Process ID to signal,required"`
	Signal string `json:"signal,omitempty" jsonschema:"description=Signal name: TERM (default)\\, KILL\\, HUP\\, INT\\, USR1\\, USR2\\, STOP\\, CONT"`
}

type ProcKillOutput struct {
	PID    int    `json:"pid"`
	Signal string `json:"signal"`
	Result string `json:"result"`
}

var procValidSignals = map[string]bool{
	"TERM": true, "KILL": true, "HUP": true, "INT": true,
	"USR1": true, "USR2": true, "STOP": true, "CONT": true,
}

type ProcPortListInput struct {
	Port int `json:"port,omitempty" jsonschema:"description=Filter by specific port number"`
}

type ProcPortEntry struct {
	Protocol string `json:"protocol"`
	Address  string `json:"address"`
	Port     int    `json:"port"`
	Process  string `json:"process"`
}

type ProcPortListOutput struct {
	Ports []ProcPortEntry `json:"ports"`
	Total int             `json:"total"`
}

type ProcGpuStatusInput struct{}

type ProcGpuInfo struct {
	DriverVersion string  `json:"driver_version"`
	Name          string  `json:"name"`
	Temperature   int     `json:"temperature"`
	Utilization   int     `json:"utilization"`
	MemoryUsed    int     `json:"memory_used_mb"`
	MemoryTotal   int     `json:"memory_total_mb"`
	PowerDraw     float64 `json:"power_draw_w"`
}

type ProcGpuProcess struct {
	PID        int    `json:"pid"`
	Name       string `json:"name"`
	MemoryUsed int    `json:"memory_used_mb"`
}

type ProcGpuStatusOutput struct {
	GPU       *ProcGpuInfo     `json:"gpu,omitempty"`
	Processes []ProcGpuProcess `json:"processes"`
}

type ProcSystemInfoInput struct{}

type ProcSystemInfoOutput struct {
	Hostname    string    `json:"hostname"`
	Kernel      string    `json:"kernel"`
	Uptime      string    `json:"uptime"`
	LoadAvg     []float64 `json:"load_avg"`
	CPUCount    int       `json:"cpu_count"`
	MemTotalMB  int       `json:"mem_total_mb"`
	MemAvailMB  int       `json:"mem_available_mb"`
	SwapTotalMB int       `json:"swap_total_mb"`
	SwapUsedMB  int       `json:"swap_used_mb"`
}

type ProcInvestigatePortInput struct {
	Port     int `json:"port" jsonschema:"required,description=TCP port number to investigate"`
	LogLines int `json:"log_lines,omitempty" jsonschema:"description=Number of journal log lines to fetch. Default 20."`
}

type ProcInvestigatePortOutput struct {
	Port          int              `json:"port"`
	Process       *ProcProcessInfo `json:"process,omitempty"`
	Tree          string           `json:"tree,omitempty"`
	SystemdUnit   string           `json:"systemd_unit,omitempty"`
	SystemdStatus string           `json:"systemd_status,omitempty"`
	RecentLogs    string           `json:"recent_logs,omitempty"`
}

type ProcInvestigateServiceInput struct {
	Unit     string `json:"unit" jsonschema:"required,description=Systemd unit name to investigate"`
	System   bool   `json:"system,omitempty" jsonschema:"description=Target system scope instead of user scope. Default: user scope."`
	LogLines int    `json:"log_lines,omitempty" jsonschema:"description=Number of journal log lines to fetch. Default 20."`
}

type ProcInvestigateServiceOutput struct {
	Unit        string           `json:"unit"`
	ActiveState string           `json:"active_state"`
	SubState    string           `json:"sub_state"`
	MainPID     int              `json:"main_pid,omitempty"`
	Process     *ProcProcessInfo `json:"process,omitempty"`
	Ports       []ProcPortEntry  `json:"ports,omitempty"`
	RecentLogs  string           `json:"recent_logs,omitempty"`
}

// ---------------------------------------------------------------------------
// Module
// ---------------------------------------------------------------------------

// ProcessModule provides process management, port inspection, GPU, and system info tools.
type ProcessModule struct{}

func (m *ProcessModule) Name() string        { return "process" }
func (m *ProcessModule) Description() string { return "Process management: list, tree, kill, ports, GPU status, system info" }

func (m *ProcessModule) Tools() []registry.ToolDefinition {
	// ── ps_list ──────────────────────────────────────────
	psList := handler.TypedHandler[ProcPsListInput, ProcPsListOutput](
		"ps_list",
		"List running processes sorted by CPU, memory, or PID. Optionally filter by command substring.",
		func(ctx context.Context, input ProcPsListInput) (ProcPsListOutput, error) {
			sortFlag := "--sort=-pcpu"
			switch input.SortBy {
			case "mem":
				sortFlag = "--sort=-pmem"
			case "pid":
				sortFlag = "--sort=pid"
			case "cpu", "":
			default:
				return ProcPsListOutput{}, fmt.Errorf("[%s] sort_by must be cpu, mem, or pid", handler.ErrInvalidParam)
			}

			limit := input.Limit
			if limit <= 0 {
				limit = 20
			}

			out, _, err := procRunCmd(ctx, "ps", "aux", sortFlag)
			if err != nil {
				return ProcPsListOutput{}, fmt.Errorf("[%s] ps command failed: %w", handler.ErrAPIError, err)
			}

			lines := strings.Split(out, "\n")
			var processes []ProcProcessInfo
			for i, line := range lines {
				if i == 0 {
					continue
				}
				fields := strings.Fields(line)
				if len(fields) < 11 {
					continue
				}

				command := strings.Join(fields[10:], " ")
				if input.Filter != "" && !strings.Contains(strings.ToLower(command), strings.ToLower(input.Filter)) {
					continue
				}

				pid, _ := strconv.Atoi(fields[1])
				cpu, _ := strconv.ParseFloat(fields[2], 64)
				mem, _ := strconv.ParseFloat(fields[3], 64)
				vsz, _ := strconv.Atoi(fields[4])
				rss, _ := strconv.Atoi(fields[5])

				processes = append(processes, ProcProcessInfo{
					User:    fields[0],
					PID:     pid,
					CPU:     cpu,
					Mem:     mem,
					VSZ:     vsz,
					RSS:     rss,
					TTY:     fields[6],
					Stat:    fields[7],
					Start:   fields[8],
					Time:    fields[9],
					Command: command,
				})

				if len(processes) >= limit {
					break
				}
			}

			if processes == nil {
				processes = []ProcProcessInfo{}
			}
			return ProcPsListOutput{Processes: processes, Total: len(processes)}, nil
		},
	)
	psList.SearchTerms = []string{"top processes", "running processes", "cpu usage", "memory usage"}

	// ── ps_tree ──────────────────────────────────────────
	psTree := handler.TypedHandler[ProcPsTreeInput, ProcPsTreeOutput](
		"ps_tree",
		"Show process tree for a given PID using pstree. Falls back to ps --forest if pstree is unavailable.",
		func(ctx context.Context, input ProcPsTreeInput) (ProcPsTreeOutput, error) {
			if input.PID <= 0 {
				return ProcPsTreeOutput{}, fmt.Errorf("[%s] pid must be a positive integer", handler.ErrInvalidParam)
			}

			pidStr := strconv.Itoa(input.PID)
			out, _, err := procRunCmd(ctx, "pstree", "-p", pidStr)
			if err == nil {
				return ProcPsTreeOutput{PID: input.PID, Tree: out}, nil
			}

			out, _, err = procRunCmd(ctx, "ps", "-ef", "--forest")
			if err != nil {
				return ProcPsTreeOutput{}, fmt.Errorf("[%s] ps --forest failed: %w", handler.ErrAPIError, err)
			}

			var filtered []string
			for line := range strings.SplitSeq(out, "\n") {
				if strings.Contains(line, pidStr) {
					filtered = append(filtered, line)
				}
			}
			if len(filtered) == 0 {
				return ProcPsTreeOutput{}, fmt.Errorf("[%s] process not found: pid %d", handler.ErrNotFound, input.PID)
			}

			return ProcPsTreeOutput{PID: input.PID, Tree: strings.Join(filtered, "\n")}, nil
		},
	)
	psTree.MaxResultChars = 8000

	// ── kill_process ─────────────────────────────────────
	killProcess := handler.TypedHandler[ProcKillInput, ProcKillOutput](
		"kill_process",
		"Send a signal to a process. Supports TERM, KILL, HUP, INT, USR1, USR2, STOP, CONT.",
		func(ctx context.Context, input ProcKillInput) (ProcKillOutput, error) {
			if input.PID <= 0 {
				return ProcKillOutput{}, fmt.Errorf("[%s] pid must be a positive integer", handler.ErrInvalidParam)
			}

			sig := input.Signal
			if sig == "" {
				sig = "TERM"
			}
			sig = strings.ToUpper(sig)
			if !procValidSignals[sig] {
				return ProcKillOutput{}, fmt.Errorf("[%s] invalid signal %q; must be one of: TERM, KILL, HUP, INT, USR1, USR2, STOP, CONT", handler.ErrInvalidParam, sig)
			}

			slog.Info("sending signal", "pid", input.PID, "signal", sig)
			pidStr := strconv.Itoa(input.PID)
			_, stderr, err := procRunCmd(ctx, "kill", "-"+sig, pidStr)
			if err != nil {
				if strings.Contains(stderr, "No such process") {
					slog.Error("process not found", "pid", input.PID, "signal", sig)
					return ProcKillOutput{}, fmt.Errorf("[%s] process not found: pid %d", handler.ErrNotFound, input.PID)
				}
				if strings.Contains(stderr, "Operation not permitted") {
					slog.Error("permission denied", "pid", input.PID, "signal", sig)
					return ProcKillOutput{}, fmt.Errorf("[%s] permission denied: pid %d", handler.ErrPermission, input.PID)
				}
				slog.Error("kill failed", "pid", input.PID, "signal", sig, "error", stderr)
				return ProcKillOutput{}, fmt.Errorf("[%s] kill failed: %s", handler.ErrAPIError, stderr)
			}

			slog.Info("signal sent", "pid", input.PID, "signal", sig)
			return ProcKillOutput{
				PID:    input.PID,
				Signal: sig,
				Result: fmt.Sprintf("sent %s to pid %d", sig, input.PID),
			}, nil
		},
	)
	killProcess.IsWrite = true

	// ── port_list ────────────────────────────────────────
	portList := handler.TypedHandler[ProcPortListInput, ProcPortListOutput](
		"port_list",
		"List listening TCP ports with process info via ss. Optionally filter by port number.",
		func(ctx context.Context, input ProcPortListInput) (ProcPortListOutput, error) {
			out, _, err := procRunCmd(ctx, "ss", "-tlnp")
			if err != nil {
				return ProcPortListOutput{}, fmt.Errorf("[%s] ss command failed: %w", handler.ErrAPIError, err)
			}

			lines := strings.Split(out, "\n")
			var ports []ProcPortEntry
			for i, line := range lines {
				if i < 1 {
					continue
				}
				fields := strings.Fields(line)
				if len(fields) < 5 || fields[0] != "LISTEN" {
					continue
				}

				localAddr := fields[3]
				lastColon := strings.LastIndex(localAddr, ":")
				if lastColon < 0 {
					continue
				}

				addr := localAddr[:lastColon]
				portNum, err := strconv.Atoi(localAddr[lastColon+1:])
				if err != nil {
					continue
				}
				if input.Port > 0 && portNum != input.Port {
					continue
				}

				process := ""
				for _, f := range fields {
					if strings.HasPrefix(f, "users:") {
						process = f
						break
					}
				}

				ports = append(ports, ProcPortEntry{
					Protocol: "tcp",
					Address:  addr,
					Port:     portNum,
					Process:  process,
				})
			}

			if ports == nil {
				ports = []ProcPortEntry{}
			}
			return ProcPortListOutput{Ports: ports, Total: len(ports)}, nil
		},
	)
	portList.SearchTerms = []string{"open ports", "listening ports", "network sockets"}

	// ── gpu_status ───────────────────────────────────────
	gpuStatus := handler.TypedHandler[ProcGpuStatusInput, ProcGpuStatusOutput](
		"gpu_status",
		"Query NVIDIA GPU status: driver, temperature, utilization, memory, power draw, and running GPU processes.",
		func(ctx context.Context, _ ProcGpuStatusInput) (ProcGpuStatusOutput, error) {
			_, err := exec.LookPath("nvidia-smi")
			if err != nil {
				return ProcGpuStatusOutput{}, fmt.Errorf("[%s] nvidia-smi not found: install NVIDIA drivers for GPU monitoring", handler.ErrNotFound)
			}

			var result ProcGpuStatusOutput
			out, stderr, err := procRunCmd(ctx, "nvidia-smi",
				"--query-gpu=driver_version,name,temperature.gpu,utilization.gpu,memory.used,memory.total,power.draw",
				"--format=csv,noheader,nounits")
			if err != nil {
				return ProcGpuStatusOutput{}, fmt.Errorf("[%s] nvidia-smi query failed: %s", handler.ErrAPIError, stderr)
			}

			fields := strings.Split(out, ", ")
			if len(fields) >= 7 {
				temp, _ := strconv.Atoi(strings.TrimSpace(fields[2]))
				util, _ := strconv.Atoi(strings.TrimSpace(fields[3]))
				memUsed, _ := strconv.Atoi(strings.TrimSpace(fields[4]))
				memTotal, _ := strconv.Atoi(strings.TrimSpace(fields[5]))
				power, _ := strconv.ParseFloat(strings.TrimSpace(fields[6]), 64)

				result.GPU = &ProcGpuInfo{
					DriverVersion: strings.TrimSpace(fields[0]),
					Name:          strings.TrimSpace(fields[1]),
					Temperature:   temp,
					Utilization:   util,
					MemoryUsed:    memUsed,
					MemoryTotal:   memTotal,
					PowerDraw:     power,
				}
			}

			out, _, err = procRunCmd(ctx, "nvidia-smi", "--query-compute-apps=pid,name,used_memory", "--format=csv,noheader,nounits")
			if err == nil && out != "" {
				for line := range strings.SplitSeq(out, "\n") {
					pfields := strings.Split(line, ", ")
					if len(pfields) >= 3 {
						pid, _ := strconv.Atoi(strings.TrimSpace(pfields[0]))
						mem, _ := strconv.Atoi(strings.TrimSpace(pfields[2]))
						result.Processes = append(result.Processes, ProcGpuProcess{
							PID:        pid,
							Name:       strings.TrimSpace(pfields[1]),
							MemoryUsed: mem,
						})
					}
				}
			}

			if result.Processes == nil {
				result.Processes = []ProcGpuProcess{}
			}
			return result, nil
		},
	)
	gpuStatus.SearchTerms = []string{"nvidia", "gpu usage", "gpu processes", "cuda processes"}

	// ── system_info ──────────────────────────────────────
	systemInfo := handler.TypedHandler[ProcSystemInfoInput, ProcSystemInfoOutput](
		"system_info",
		"Show system information: hostname, kernel, uptime, load average, CPU count, memory, and swap.",
		func(_ context.Context, _ ProcSystemInfoInput) (ProcSystemInfoOutput, error) {
			var info ProcSystemInfoOutput
			info.Hostname, _ = os.Hostname()

			if ver, err := procReadProcFile("version"); err == nil {
				fields := strings.Fields(ver)
				if len(fields) >= 3 {
					info.Kernel = fields[2]
				}
			}
			if raw, err := procReadProcFile("uptime"); err == nil {
				fields := strings.Fields(raw)
				if len(fields) >= 1 {
					secs, _ := strconv.ParseFloat(fields[0], 64)
					totalSecs := int(secs)
					days := totalSecs / 86400
					hours := (totalSecs % 86400) / 3600
					mins := (totalSecs % 3600) / 60
					info.Uptime = fmt.Sprintf("%dd %dh %dm", days, hours, mins)
				}
			}
			if raw, err := procReadProcFile("loadavg"); err == nil {
				fields := strings.Fields(raw)
				if len(fields) >= 3 {
					info.LoadAvg = make([]float64, 3)
					info.LoadAvg[0], _ = strconv.ParseFloat(fields[0], 64)
					info.LoadAvg[1], _ = strconv.ParseFloat(fields[1], 64)
					info.LoadAvg[2], _ = strconv.ParseFloat(fields[2], 64)
				}
			}
			if raw, err := procReadProcFile("cpuinfo"); err == nil {
				for line := range strings.SplitSeq(raw, "\n") {
					if strings.HasPrefix(line, "processor") {
						info.CPUCount++
					}
				}
			}
			if raw, err := procReadProcFile("meminfo"); err == nil {
				memMap := make(map[string]int)
				for line := range strings.SplitSeq(raw, "\n") {
					if strings.HasPrefix(line, "MemTotal:") ||
						strings.HasPrefix(line, "MemAvailable:") ||
						strings.HasPrefix(line, "SwapTotal:") ||
						strings.HasPrefix(line, "SwapFree:") {
						parts := strings.Fields(line)
						if len(parts) >= 2 {
							val, _ := strconv.Atoi(parts[1])
							key := strings.TrimSuffix(parts[0], ":")
							memMap[key] = val
						}
					}
				}
				info.MemTotalMB = memMap["MemTotal"] / 1024
				info.MemAvailMB = memMap["MemAvailable"] / 1024
				info.SwapTotalMB = memMap["SwapTotal"] / 1024
				info.SwapUsedMB = (memMap["SwapTotal"] - memMap["SwapFree"]) / 1024
			}
			return info, nil
		},
	)

	// ── investigate_port ─────────────────────────────────
	investigatePort := handler.TypedHandler[ProcInvestigatePortInput, ProcInvestigatePortOutput](
		"investigate_port",
		"Investigate a TCP port: find the listening process, show its tree, check systemd unit status, and fetch recent logs. Single tool replaces port_list + ps_list + systemd_status + systemd_logs.",
		func(ctx context.Context, input ProcInvestigatePortInput) (ProcInvestigatePortOutput, error) {
			if input.Port <= 0 || input.Port > 65535 {
				return ProcInvestigatePortOutput{}, fmt.Errorf("[%s] port must be 1-65535", handler.ErrInvalidParam)
			}
			logLines := input.LogLines
			if logLines <= 0 {
				logLines = 20
			}

			result := ProcInvestigatePortOutput{Port: input.Port}
			ssOut, _, _ := procRunCmd(ctx, "ss", "-tlnp", fmt.Sprintf("sport = :%d", input.Port))
			var pid int
			for line := range strings.SplitSeq(ssOut, "\n") {
				if !strings.Contains(line, "LISTEN") {
					continue
				}
				if _, after, ok := strings.Cut(line, "pid="); ok {
					pidStr := after
					if end := strings.IndexAny(pidStr, ",)"); end > 0 {
						pid, _ = strconv.Atoi(pidStr[:end])
					}
				}
			}
			if pid == 0 {
				return result, fmt.Errorf("[%s] no process listening on port %d", handler.ErrNotFound, input.Port)
			}

			psOut, _, _ := procRunCmd(ctx, "ps", "-p", strconv.Itoa(pid), "-o", "user,pid,pcpu,pmem,vsz,rss,tty,stat,start,time,command", "--no-headers")
			if psOut != "" {
				fields := strings.Fields(psOut)
				if len(fields) >= 11 {
					cpuVal, _ := strconv.ParseFloat(fields[2], 64)
					memVal, _ := strconv.ParseFloat(fields[3], 64)
					vszVal, _ := strconv.Atoi(fields[4])
					rssVal, _ := strconv.Atoi(fields[5])
					result.Process = &ProcProcessInfo{
						User:    fields[0],
						PID:     pid,
						CPU:     cpuVal,
						Mem:     memVal,
						VSZ:     vszVal,
						RSS:     rssVal,
						TTY:     fields[6],
						Stat:    fields[7],
						Start:   fields[8],
						Time:    fields[9],
						Command: strings.Join(fields[10:], " "),
					}
				}
			}

			treeOut, _, err := procRunCmd(ctx, "pstree", "-p", strconv.Itoa(pid))
			if err == nil {
				result.Tree = treeOut
			}

			unitOut, _, _ := procRunCmd(ctx, "systemctl", "--user", "status", strconv.Itoa(pid))
			if unitOut != "" {
				for line := range strings.SplitSeq(unitOut, "\n") {
					line = strings.TrimSpace(line)
					if strings.HasSuffix(line, ".service") || strings.Contains(line, ".service ") {
						parts := strings.FieldsSeq(line)
						for p := range parts {
							if strings.HasSuffix(p, ".service") {
								result.SystemdUnit = p
								break
							}
						}
						if result.SystemdUnit != "" {
							break
						}
					}
				}
				result.SystemdStatus = unitOut
			}
			if result.SystemdUnit != "" {
				logsOut, _, _ := procRunCmd(ctx, "journalctl", "--user-unit", result.SystemdUnit, "-n", strconv.Itoa(logLines), "--no-pager")
				result.RecentLogs = logsOut
			}

			return result, nil
		},
	)
	investigatePort.MaxResultChars = 9000
	investigatePort.SearchTerms = []string{"debug port", "what is using this port", "port investigation"}

	// ── investigate_service ───────────────────────────────
	investigateService := handler.TypedHandler[ProcInvestigateServiceInput, ProcInvestigateServiceOutput](
		"investigate_service",
		"Investigate a systemd service: get status, find its processes, check its ports, and fetch recent logs. Single tool replaces systemd_status + ps_list + port_list + systemd_logs.",
		func(ctx context.Context, input ProcInvestigateServiceInput) (ProcInvestigateServiceOutput, error) {
			if input.Unit == "" {
				return ProcInvestigateServiceOutput{}, fmt.Errorf("[%s] unit is required", handler.ErrInvalidParam)
			}
			logLines := input.LogLines
			if logLines <= 0 {
				logLines = 20
			}

			scope := "--user"
			journalFlag := "--user-unit"
			if input.System {
				scope = ""
				journalFlag = "-u"
			}

			result := ProcInvestigateServiceOutput{Unit: input.Unit}
			var statusArgs []string
			if scope != "" {
				statusArgs = append(statusArgs, scope)
			}
			statusArgs = append(statusArgs, "show", "--property=ActiveState,SubState,MainPID", input.Unit)
			statusOut, _, _ := procRunCmd(ctx, "systemctl", statusArgs...)
			for line := range strings.SplitSeq(statusOut, "\n") {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) != 2 {
					continue
				}
				switch parts[0] {
				case "ActiveState":
					result.ActiveState = parts[1]
				case "SubState":
					result.SubState = parts[1]
				case "MainPID":
					result.MainPID, _ = strconv.Atoi(parts[1])
				}
			}

			if result.MainPID > 0 {
				psOut, _, _ := procRunCmd(ctx, "ps", "-p", strconv.Itoa(result.MainPID), "-o", "user,pid,pcpu,pmem,vsz,rss,tty,stat,start,time,command", "--no-headers")
				if psOut != "" {
					fields := strings.Fields(psOut)
					if len(fields) >= 11 {
						cpuVal, _ := strconv.ParseFloat(fields[2], 64)
						memVal, _ := strconv.ParseFloat(fields[3], 64)
						vszVal, _ := strconv.Atoi(fields[4])
						rssVal, _ := strconv.Atoi(fields[5])
						result.Process = &ProcProcessInfo{
							User:    fields[0],
							PID:     result.MainPID,
							CPU:     cpuVal,
							Mem:     memVal,
							VSZ:     vszVal,
							RSS:     rssVal,
							TTY:     fields[6],
							Stat:    fields[7],
							Start:   fields[8],
							Time:    fields[9],
							Command: strings.Join(fields[10:], " "),
						}
					}
				}

				ssOut, _, _ := procRunCmd(ctx, "ss", "-tlnp")
				pidStr := strconv.Itoa(result.MainPID)
				for line := range strings.SplitSeq(ssOut, "\n") {
					if !strings.Contains(line, pidStr) {
						continue
					}
					fields := strings.Fields(line)
					if len(fields) < 4 || fields[0] != "LISTEN" {
						continue
					}
					localAddr := fields[3]
					lastColon := strings.LastIndex(localAddr, ":")
					if lastColon < 0 {
						continue
					}
					portNum, err := strconv.Atoi(localAddr[lastColon+1:])
					if err != nil {
						continue
					}
					result.Ports = append(result.Ports, ProcPortEntry{
						Protocol: "tcp",
						Address:  localAddr[:lastColon],
						Port:     portNum,
					})
				}
			}

			if result.Ports == nil {
				result.Ports = []ProcPortEntry{}
			}

			logsOut, _, _ := procRunCmd(ctx, "journalctl", journalFlag, input.Unit, "-n", strconv.Itoa(logLines), "--no-pager")
			result.RecentLogs = logsOut
			return result, nil
		},
	)
	investigateService.MaxResultChars = 9000
	investigateService.SearchTerms = []string{"debug service", "service investigation", "why is my service failing"}

	return []registry.ToolDefinition{
		psList,
		psTree,
		killProcess,
		portList,
		gpuStatus,
		systemInfo,
		investigatePort,
		investigateService,
	}
}
