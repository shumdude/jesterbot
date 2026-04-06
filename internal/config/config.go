package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultDBPath         = "data/jesterbot.db"
	defaultTickInterval   = time.Minute
	defaultPollTimeout    = 10 * time.Second
	defaultWorkerCount    = 4
	defaultReminderMinute = 30
)

type Config struct {
	BotToken               string
	DBPath                 string
	TickInterval           time.Duration
	PollTimeout            time.Duration
	WorkerCount            int
	DefaultReminderMinutes int
}

func Load() (Config, error) {
	if err := loadDotEnvFiles(); err != nil {
		return Config{}, err
	}

	cfg := Config{
		BotToken:               os.Getenv("JESTERBOT_BOT_TOKEN"),
		DBPath:                 envString("JESTERBOT_DB_PATH", defaultDBPath),
		TickInterval:           envDuration("JESTERBOT_TICK_INTERVAL", defaultTickInterval),
		PollTimeout:            envDuration("JESTERBOT_POLL_TIMEOUT", defaultPollTimeout),
		WorkerCount:            envInt("JESTERBOT_WORKERS", defaultWorkerCount),
		DefaultReminderMinutes: envInt("JESTERBOT_DEFAULT_REMINDER_MINUTES", defaultReminderMinute),
	}

	if cfg.BotToken == "" {
		return Config{}, errors.New("JESTERBOT_BOT_TOKEN is required")
	}

	if cfg.WorkerCount <= 0 {
		return Config{}, fmt.Errorf("JESTERBOT_WORKERS must be positive, got %d", cfg.WorkerCount)
	}

	if cfg.DefaultReminderMinutes <= 0 {
		return Config{}, fmt.Errorf("JESTERBOT_DEFAULT_REMINDER_MINUTES must be positive, got %d", cfg.DefaultReminderMinutes)
	}

	return cfg, nil
}

func envString(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func loadDotEnvFiles() error {
	startDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	initialEnv := snapshotEnvPresence()
	paths, err := dotEnvPaths(startDir)
	if err != nil {
		return err
	}

	loadedValues := make(map[string]string)
	for _, path := range paths {
		fileValues, err := readDotEnvFile(path)
		if err != nil {
			return err
		}
		for key, value := range fileValues {
			loadedValues[key] = value
		}
	}

	for key, value := range loadedValues {
		if initialEnv[key] {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set %s from .env: %w", key, err)
		}
	}

	return nil
}

func snapshotEnvPresence() map[string]bool {
	presence := make(map[string]bool)
	for _, entry := range os.Environ() {
		key, _, ok := strings.Cut(entry, "=")
		if ok {
			presence[key] = true
		}
	}
	return presence
}

func dotEnvPaths(startDir string) ([]string, error) {
	dirs := make([]string, 0, 4)
	dir := filepath.Clean(startDir)

	for {
		dirs = append(dirs, dir)

		if fileExists(filepath.Join(dir, "go.mod")) {
			break
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	paths := make([]string, 0, len(dirs))
	for i := len(dirs) - 1; i >= 0; i-- {
		path := filepath.Join(dirs[i], ".env")
		if fileExists(path) {
			paths = append(paths, path)
		}
	}

	return paths, nil
}

func readDotEnvFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("parse %s:%d: expected KEY=VALUE", path, lineNumber)
		}

		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("parse %s:%d: empty key", path, lineNumber)
		}

		values[key] = parseDotEnvValue(value)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan %s: %w", path, err)
	}

	return values, nil
}

func parseDotEnvValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) >= 2 {
		if strings.HasPrefix(trimmed, "\"") && strings.HasSuffix(trimmed, "\"") {
			unquoted, err := strconv.Unquote(trimmed)
			if err == nil {
				return unquoted
			}
		}
		if strings.HasPrefix(trimmed, "'") && strings.HasSuffix(trimmed, "'") {
			return trimmed[1 : len(trimmed)-1]
		}
	}

	if commentIndex := strings.Index(trimmed, " #"); commentIndex >= 0 {
		trimmed = strings.TrimSpace(trimmed[:commentIndex])
	}

	return trimmed
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
