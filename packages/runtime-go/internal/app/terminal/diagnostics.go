package terminal

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"analytix.local/runtime-go/internal/ports"
)

func CommandDiagnostics(probe ports.CommandProbe, homeDir string) []any {
	return CommandDiagnosticsFor([]string{
		"go version",
		"node --version",
		"npm --version",
		"git --version",
		"rg --version",
		"rustc --version",
		"cargo --version",
		"make --version",
		"docker --version",
		"python3 --version",
		"python --version",
	}, 2*time.Second, probe, homeDir)
}

func CommandDiagnosticsFor(commands []string, timeout time.Duration, probe ports.CommandProbe, homeDir string) []any {
	results := make([]map[string]any, len(commands))
	var wg sync.WaitGroup
	for index, command := range commands {
		wg.Add(1)
		go func(index int, command string) {
			defer wg.Done()
			results[index] = CommandDiagnostic(command, timeout, probe, homeDir)
		}(index, command)
	}
	wg.Wait()
	sort.SliceStable(results, func(i, j int) bool {
		leftFound, rightFound := boolValue(results[i]["found"]), boolValue(results[j]["found"])
		if leftFound != rightFound {
			return leftFound
		}
		return stringValue(results[i]["binary"]) < stringValue(results[j]["binary"])
	})
	out := make([]any, 0, len(results))
	for _, result := range results {
		out = append(out, result)
	}
	return out
}

func CommandDiagnostic(command string, timeout time.Duration, probe ports.CommandProbe, homeDir string) map[string]any {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return map[string]any{
			"command": command,
			"binary":  command,
			"found":   false,
			"error":   "empty command",
		}
	}
	binary := parts[0]
	record := map[string]any{
		"command": command,
		"binary":  binary,
	}
	if probe == nil {
		record["found"] = false
		record["error"] = "command probe unavailable"
		return record
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	result := probe.ProbeCommand(ctx, ports.CommandProbeRequest{Binary: binary, Args: parts[1:]})
	if !result.Found {
		record["found"] = false
		record["error"] = firstNonEmptyDiagnosticString(result.Error, "not found")
		return record
	}
	output := CommandOutputLine(result.Stdout, homeDir)
	if output == "" {
		output = CommandOutputLine(result.Stderr, homeDir)
	}
	if result.Error != "" || result.TimedOut {
		record["found"] = false
		if result.TimedOut || ctx.Err() == context.DeadlineExceeded {
			record["error"] = "timeout"
		} else if output != "" {
			record["error"] = output
		} else {
			record["error"] = CommandOutputLine(result.Error, homeDir)
		}
		return record
	}
	record["found"] = true
	if output == "" {
		output = "available"
	}
	record["output"] = output
	return record
}

func CommandOutputLine(value string, homeDir string) string {
	text := regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`).ReplaceAllString(value, "")
	if homeDir != "" {
		text = strings.ReplaceAll(text, homeDir, "~")
	}
	text = strings.TrimSpace(text)
	if index := strings.Index(text, "\n"); index >= 0 {
		text = strings.TrimSuffix(text[:index], "\r")
	}
	runes := []rune(strings.TrimSpace(text))
	if len(runes) > 240 {
		return strings.TrimSpace(string(runes[:237])) + "..."
	}
	return string(runes)
}

func firstNonEmptyDiagnosticString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func boolValue(value any) bool {
	boolean, _ := value.(bool)
	return boolean
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}
