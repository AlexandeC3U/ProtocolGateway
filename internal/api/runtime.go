package api

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

var containerNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`)

type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Source    string `json:"source"`
	Message   string `json:"message"`
	Raw       string `json:"raw"`
}

type LogProvider interface {
	ListContainers(ctx context.Context) ([]string, error)
	TailLogs(ctx context.Context, container string, tail int) ([]LogEntry, error)
}

// DockerCLILogProvider uses the local docker CLI to fetch container names/logs.
// Intended for dev/operator environments where the gateway runs on the host.
type DockerCLILogProvider struct {
	logger zerolog.Logger
}

func NewDockerCLILogProvider(logger zerolog.Logger) *DockerCLILogProvider {
	return &DockerCLILogProvider{logger: logger.With().Str("component", "docker-logs").Logger()}
}

func (p *DockerCLILogProvider) ListContainers(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, "docker", "ps", "--format", "{{.Names}}")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker ps failed: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	containers := make([]string, 0, len(lines))
	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		containers = append(containers, name)
	}
	return containers, nil
}

func (p *DockerCLILogProvider) TailLogs(ctx context.Context, container string, tail int) ([]LogEntry, error) {
	if !containerNameRe.MatchString(container) {
		return nil, fmt.Errorf("invalid container name")
	}
	if tail <= 0 {
		tail = 300
	}
	if tail > 5000 {
		tail = 5000
	}

	cmd := exec.CommandContext(ctx, "docker", "logs", "--timestamps", "--tail", strconv.Itoa(tail), container)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker logs failed: %w", err)
	}

	entries := make([]LogEntry, 0, tail)
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		entries = append(entries, parseLogLine(line))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to parse docker logs: %w", err)
	}
	return entries, nil
}

func parseLogLine(line string) LogEntry {
	// docker --timestamps prefixes RFC3339Nano timestamp.
	// Example: 2026-01-26T15:05:26.123456789Z 2026-01-26 16:05:26 ... INF msg
	fields := strings.Fields(line)
	if len(fields) >= 2 {
		if _, err := time.Parse(time.RFC3339Nano, fields[0]); err == nil {
			ts := fields[0]
			rest := strings.TrimSpace(strings.TrimPrefix(line, fields[0]))
			parsed := parseGatewayStyle(rest)
			if parsed.Timestamp == "" {
				parsed.Timestamp = ts
			}
			parsed.Raw = line
			return parsed
		}
	}

	parsed := parseGatewayStyle(line)
	parsed.Raw = line
	return parsed
}

func parseGatewayStyle(s string) LogEntry {
	// Best-effort parse of console logs like:
	// 2026-01-26 16:05:26 2026-01-26T15:05:26Z INF Message key=value ...
	parts := strings.Fields(s)
	if len(parts) >= 4 {
		// local date + time
		localTs := parts[0] + " " + parts[1]
		level := parts[3]
		msg := strings.TrimSpace(strings.Join(parts[4:], " "))
		source := extractSource(msg)
		return LogEntry{Timestamp: localTs, Level: level, Source: source, Message: msg, Raw: s}
	}

	// Fallback: try to find level token
	level := ""
	for _, tok := range parts {
		switch tok {
		case "TRC", "DBG", "INF", "WRN", "ERR", "FTL":
			level = tok
		}
	}
	return LogEntry{Level: level, Source: extractSource(s), Message: strings.TrimSpace(s), Raw: s}
}

func extractSource(msg string) string {
	// Prefer component=..., otherwise service=...
	if v := extractKV(msg, "component"); v != "" {
		return v
	}
	if v := extractKV(msg, "service"); v != "" {
		return v
	}
	return ""
}

func extractKV(s, key string) string {
	needle := key + "="
	idx := strings.Index(s, needle)
	if idx < 0 {
		return ""
	}
	start := idx + len(needle)
	if start >= len(s) {
		return ""
	}
	// value may be quoted or unquoted
	if s[start] == '"' {
		end := strings.Index(s[start+1:], "\"")
		if end < 0 {
			return ""
		}
		return s[start+1 : start+1+end]
	}
	end := start
	for end < len(s) && s[end] != ' ' {
		end++
	}
	return s[start:end]
}
