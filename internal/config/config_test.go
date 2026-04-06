package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReadsDotEnvFromWorkingDirectoryChain(t *testing.T) {
	unsetEnvKeys(t,
		"JESTERBOT_BOT_TOKEN",
		"JESTERBOT_DB_PATH",
		"JESTERBOT_WORKERS",
	)

	rootDir := t.TempDir()
	writeTestFile(t, filepath.Join(rootDir, "go.mod"), "module example\n")
	writeTestFile(t, filepath.Join(rootDir, ".env"), "JESTERBOT_BOT_TOKEN=root-token\nJESTERBOT_DB_PATH=root.db\n")

	cmdDir := filepath.Join(rootDir, "cmd")
	writeTestFile(t, filepath.Join(cmdDir, ".env"), "JESTERBOT_DB_PATH=cmd.db\n")

	workDir := filepath.Join(cmdDir, "jesterbot")
	writeTestFile(t, filepath.Join(workDir, ".env"), "JESTERBOT_BOT_TOKEN=child-token\nJESTERBOT_WORKERS=7\n")

	restoreWorkingDir(t, workDir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.BotToken != "child-token" {
		t.Fatalf("unexpected bot token: %q", cfg.BotToken)
	}
	if cfg.DBPath != "cmd.db" {
		t.Fatalf("unexpected db path: %q", cfg.DBPath)
	}
	if cfg.WorkerCount != 7 {
		t.Fatalf("unexpected worker count: %d", cfg.WorkerCount)
	}
}

func TestLoadPrefersProcessEnvironmentOverDotEnv(t *testing.T) {
	unsetEnvKeys(t, "JESTERBOT_BOT_TOKEN")

	rootDir := t.TempDir()
	writeTestFile(t, filepath.Join(rootDir, "go.mod"), "module example\n")
	writeTestFile(t, filepath.Join(rootDir, ".env"), "JESTERBOT_BOT_TOKEN=file-token\n")
	restoreWorkingDir(t, rootDir)

	if err := os.Setenv("JESTERBOT_BOT_TOKEN", "process-token"); err != nil {
		t.Fatalf("set env: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.BotToken != "process-token" {
		t.Fatalf("unexpected bot token: %q", cfg.BotToken)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func restoreWorkingDir(t *testing.T, dir string) {
	t.Helper()

	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("change working directory to %s: %v", dir, err)
	}

	t.Cleanup(func() {
		if err := os.Chdir(previousDir); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})
}

func unsetEnvKeys(t *testing.T, keys ...string) {
	t.Helper()

	restoreValues := make(map[string]*string, len(keys))
	for _, key := range keys {
		value, ok := os.LookupEnv(key)
		if ok {
			valueCopy := value
			restoreValues[key] = &valueCopy
		} else {
			restoreValues[key] = nil
		}

		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset %s: %v", key, err)
		}
	}

	t.Cleanup(func() {
		for _, key := range keys {
			value := restoreValues[key]
			var err error
			if value == nil {
				err = os.Unsetenv(key)
			} else {
				err = os.Setenv(key, *value)
			}
			if err != nil {
				t.Fatalf("restore %s: %v", key, err)
			}
		}
	})
}
