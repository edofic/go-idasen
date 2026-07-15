package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const version = "0.2.0"

var reserved = map[string]bool{"init": true, "pair": true, "tray": true, "controller": true, "monitor": true, "height": true, "speed": true, "move": true, "save": true, "delete": true}

type Config struct {
	MACAddress string             `yaml:"mac_address"`
	Positions  map[string]float64 `yaml:"positions"`
}

type countFlag int

func (c *countFlag) String() string { return fmt.Sprint(int(*c)) }
func (c *countFlag) Set(string) error {
	*c++
	return nil
}
func (c *countFlag) IsBoolFlag() bool { return true }

func configPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if !filepath.IsAbs(base) {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "idasen", "idasen.yaml")
}

func load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{Positions: map[string]float64{}}, nil
	}
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return Config{}, fmt.Errorf("invalid configuration: %w", err)
	}
	if cfg.Positions == nil {
		cfg.Positions = map[string]float64{}
	}
	if cfg.MACAddress != "" && len(cfg.MACAddress) < 17 {
		return Config{}, errors.New("invalid configuration: mac_address must be at least 17 characters")
	}
	for name, height := range cfg.Positions {
		if reserved[name] {
			return Config{}, fmt.Errorf("invalid configuration: position %q is a reserved name", name)
		}
		if height < MinHeight || height > MaxHeight {
			return Config{}, fmt.Errorf("invalid configuration: position %q is outside %.2f–%.2f metres", name, MinHeight, MaxHeight)
		}
	}
	return cfg, nil
}

func save(path string, cfg Config) error {
	b, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, append([]byte("# IKEA IDÅSEN desk configuration\n"), b...), 0644)
}

func usage(w io.Writer, cfg Config) {
	fmt.Fprintln(w, "Usage: idasen [--mac-address ADDRESS] [-v] <command>")
	fmt.Fprintln(w, "\nCommands: init, pair, tray, controller, height, speed, monitor, move HEIGHT, save NAME, delete NAME")
	if len(cfg.Positions) != 0 {
		names := make([]string, 0, len(cfg.Positions))
		for n := range cfg.Positions {
			names = append(names, n)
		}
		sort.Strings(names)
		fmt.Fprintf(w, "Saved positions: %s\n", strings.Join(names, ", "))
	}
}

// Run executes the command using the process arguments.
func Run() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	path := configPath()
	cfg, err := load(path)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fs := flag.NewFlagSet("idasen", flag.ContinueOnError)
	fs.SetOutput(stderr)
	mac := fs.String("mac-address", "", "desk MAC address")
	fs.StringVar(mac, "m", "", "desk MAC address")
	var verbose countFlag
	fs.Var(&verbose, "v", "increase logging verbosity")
	versionFlag := fs.Bool("version", false, "print version")
	fs.Usage = func() { usage(stderr, cfg) }
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *versionFlag {
		fmt.Fprintln(stdout, version)
		return 0
	}
	remaining := fs.Args()
	if len(remaining) == 0 {
		fmt.Fprintln(stderr, "A subcommand is required")
		usage(stderr, cfg)
		return 1
	}
	command := remaining[0]
	if *mac == "" {
		*mac = cfg.MACAddress
	}
	_ = verbose // retained for CLI compatibility; BLE errors are always returned.

	if command == "init" {
		return initialize(remaining[1:], path, cfg, stdout, stderr)
	}
	if command == "delete" {
		return deletePosition(remaining[1:], path, &cfg, stdout, stderr)
	}
	if command == "pair" {
		fmt.Fprintln(stderr, "Pairing is managed by the operating system; pair the desk with bluetoothctl or your system Bluetooth settings.")
		return 1
	}
	if *mac == "" {
		fmt.Fprintln(stderr, "mac_address must be provided via --mac-address or the config file")
		return 2
	}
	if command == "tray" {
		return runTray(*mac, cfg, stderr)
	}
	if command == "controller" {
		if len(remaining) != 1 {
			fmt.Fprintln(stderr, "Usage: idasen controller")
			return 2
		}
		return runController(*mac, path, cfg, os.Stdin, stdout, stderr)
	}
	if err := PrepareBluetooth(); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	d, err := Connect(ctx, *mac)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer d.Close()
	switch command {
	case "height":
		h, _, err := d.HeightAndSpeed()
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintf(stdout, "%.3f meters\n", h)
	case "speed":
		_, s, err := d.HeightAndSpeed()
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintf(stdout, "%.3f meters/second\n", s)
	case "save":
		if len(remaining) != 2 {
			fmt.Fprintln(stderr, "Usage: idasen save NAME")
			return 2
		}
		if reserved[remaining[1]] {
			fmt.Fprintf(stderr, "Position with name %q is a reserved name.\n", remaining[1])
			return 1
		}
		h, _, err := d.HeightAndSpeed()
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		cfg.Positions[remaining[1]] = h
		if err := save(path, cfg); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintf(stdout, "Saved position %q with height: %.3fm.\n", remaining[1], h)
	case "monitor":
		monitorCtx, stop := signalContext()
		defer stop()
		if err := d.Monitor(monitorCtx, func(h, s float64) { fmt.Fprintf(stdout, "%.3f meters - %.3f meters/second\n", h, s) }); err != nil && !errors.Is(err, context.Canceled) {
			fmt.Fprintln(stderr, err)
			return 1
		}
	case "move":
		if len(remaining) != 2 {
			fmt.Fprintln(stderr, "Usage: idasen move HEIGHT")
			return 2
		}
		target, err := strconv.ParseFloat(remaining[1], 64)
		if err != nil {
			fmt.Fprintf(stderr, "invalid height %q: %v\n", remaining[1], err)
			return 2
		}
		moveCtx, stop := signalContext()
		defer stop()
		if err := d.MoveTo(moveCtx, target); err != nil && !errors.Is(err, context.Canceled) {
			fmt.Fprintln(stderr, err)
			return 1
		}
	default:
		target, ok := cfg.Positions[command]
		if !ok {
			fmt.Fprintf(stderr, "Unknown subcommand or saved position %q\n", command)
			usage(stderr, cfg)
			return 2
		}
		moveCtx, stop := signalContext()
		defer stop()
		if err := d.MoveTo(moveCtx, target); err != nil && !errors.Is(err, context.Canceled) {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	return 0
}
