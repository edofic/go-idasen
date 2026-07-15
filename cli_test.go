package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingConfig(t *testing.T) {
	cfg, err := load(filepath.Join(t.TempDir(), "missing.yaml"))
	if err != nil || len(cfg.Positions) != 0 {
		t.Fatalf("got %#v, %v", cfg, err)
	}
}

func TestLoadRejectsReservedPosition(t *testing.T) {
	path := filepath.Join(t.TempDir(), "idasen.yaml")
	if err := os.WriteFile(path, []byte("positions:\n  height: 0.8\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := load(path); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestRunNeedsSubcommand(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 1 {
		t.Fatalf("got %d", code)
	}
}

func TestRunVersion(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("got %d: %s", code, stderr.String())
	}
	if got, want := stdout.String(), version+"\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestDeleteMissingPosition(t *testing.T) {
	path := filepath.Join(t.TempDir(), "idasen.yaml")
	var stdout, stderr bytes.Buffer
	if code := deletePosition([]string{"absent"}, path, &Config{Positions: map[string]float64{}}, &stdout, &stderr); code != 0 {
		t.Fatalf("got %d", code)
	}
}
