package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"
)

func initialize(args []string, path string, cfg Config, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(stderr)
	force := fs.Bool("force", false, "overwrite existing configuration")
	fs.BoolVar(force, "f", false, "overwrite existing configuration")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if !*force {
		if _, err := os.Stat(path); err == nil {
			fmt.Fprintln(stderr, "Configuration file already exists.\nUse --force to overwrite existing configuration.")
			return 1
		}
	}
	if len(cfg.Positions) == 0 {
		cfg.Positions = map[string]float64{"sit": .75, "stand": 1.1}
	}
	if err := PrepareBluetooth(); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if mac, err := Discover(ctx); err == nil && mac != "" {
		cfg.MACAddress = mac
		fmt.Fprintf(stderr, "Discovered desk's MAC address: %s\n", mac)
	} else {
		fmt.Fprintln(stderr, "Failed to discover desk's MAC address")
	}
	if err := save(path, cfg); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stderr, "Created new configuration file at: %s\n", path)
	fmt.Fprintln(stdout, "Pair the desk through your operating system's Bluetooth settings if needed.")
	return 0
}

func deletePosition(args []string, path string, cfg *Config, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "Usage: idasen delete NAME")
		return 2
	}
	name := args[0]
	if reserved[name] {
		fmt.Fprintf(stderr, "Position with name %q is a reserved name.\n", name)
		return 1
	}
	if _, ok := cfg.Positions[name]; !ok {
		fmt.Fprintf(stderr, "Position with name %q doesn't exist.\n", name)
		return 0
	}
	delete(cfg.Positions, name)
	if err := save(path, *cfg); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "Position with name %q removed.\n", name)
	return 0
}
