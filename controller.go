package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

const controllerPollInterval = 250 * time.Millisecond

type controllerCommand struct {
	Action   string  `json:"action"`
	Position string  `json:"position,omitempty"`
	Height   float64 `json:"height,omitempty"`
	Delta    float64 `json:"delta,omitempty"`
}

type controllerEvent struct {
	Type      string             `json:"type"`
	Status    string             `json:"status,omitempty"`
	Height    float64            `json:"height,omitempty"`
	Speed     float64            `json:"speed,omitempty"`
	Positions map[string]float64 `json:"positions,omitempty"`
	Message   string             `json:"message,omitempty"`
}

type eventWriter struct {
	mu  sync.Mutex
	enc *json.Encoder
}

func (w *eventWriter) write(event controllerEvent) {
	w.mu.Lock()
	defer w.mu.Unlock()
	_ = w.enc.Encode(event)
}

type commandResult struct {
	command controllerCommand
	err     error
}

func readControllerCommands(r io.Reader) <-chan commandResult {
	results := make(chan commandResult)
	go func() {
		defer close(results)
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			var command controllerCommand
			err := json.Unmarshal(scanner.Bytes(), &command)
			if err == nil {
				err = validateControllerCommand(command)
			}
			results <- commandResult{command: command, err: err}
		}
		if err := scanner.Err(); err != nil {
			results <- commandResult{err: err}
		}
	}()
	return results
}

func validateControllerCommand(command controllerCommand) error {
	switch command.Action {
	case "move":
		if command.Position == "" && (command.Height < MinHeight || command.Height > MaxHeight) {
			return fmt.Errorf("move height must be between %.0f and %.0f cm", MinHeight*100, MaxHeight*100)
		}
	case "adjust":
		if command.Delta == 0 || math.Abs(command.Delta) > .10 {
			return errors.New("adjust delta must be non-zero and no more than 10 cm")
		}
	case "set_position":
		if command.Position == "" {
			return errors.New("position name is required")
		}
		if reserved[command.Position] {
			return fmt.Errorf("position %q is a reserved name", command.Position)
		}
		if command.Height < MinHeight || command.Height > MaxHeight {
			return fmt.Errorf("position height must be between %.0f and %.0f cm", MinHeight*100, MaxHeight*100)
		}
	case "refresh", "stop":
	default:
		return fmt.Errorf("unknown controller action %q", command.Action)
	}
	return nil
}

func runController(mac, path string, cfg Config, stdin io.Reader, stdout, stderr io.Writer) int {
	events := &eventWriter{enc: json.NewEncoder(stdout)}
	events.write(controllerEvent{Type: "status", Status: "connecting", Positions: clonePositions(cfg.Positions)})
	if err := PrepareBluetooth(); err != nil {
		events.write(controllerEvent{Type: "error", Status: "unavailable", Message: err.Error()})
		fmt.Fprintln(stderr, err)
		return 1
	}

	ctx, cancel := signalContext()
	defer cancel()
	connectCtx, stopConnect := context.WithTimeout(ctx, 15*time.Second)
	desk, err := Connect(connectCtx, mac)
	stopConnect()
	if err != nil {
		events.write(controllerEvent{Type: "error", Status: "unavailable", Message: err.Error()})
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer desk.Close()

	var lastHeight atomic.Uint64
	publishHeight := func(height, speed float64, status string) {
		lastHeight.Store(math.Float64bits(height))
		events.write(controllerEvent{Type: "height", Status: status, Height: height, Speed: speed})
	}
	height, speed, err := desk.HeightAndSpeed()
	if err != nil {
		events.write(controllerEvent{Type: "error", Status: "unavailable", Message: err.Error()})
		fmt.Fprintln(stderr, err)
		return 1
	}
	publishHeight(height, speed, movementStatus(speed))

	commands := readControllerCommands(stdin)
	ticker := time.NewTicker(controllerPollInterval)
	defer ticker.Stop()

	type moveResult struct{ err error }
	moveDone := make(chan moveResult, 1)
	var moveCancel context.CancelFunc
	var pendingTarget *float64
	moving := false
	startMove := func(target float64) {
		moveCtx, stop := context.WithTimeout(ctx, 2*time.Minute)
		moveCancel = stop
		moving = true
		events.write(controllerEvent{Type: "status", Status: "moving"})
		go func() {
			moveDone <- moveResult{err: desk.moveTo(moveCtx, target, func(height, speed float64) {
				publishHeight(height, speed, "moving")
			})}
		}()
	}

	for {
		select {
		case <-ctx.Done():
			if moveCancel != nil {
				moveCancel()
				<-moveDone
			}
			return 0
		case result, ok := <-commands:
			if !ok {
				if moveCancel != nil {
					moveCancel()
					<-moveDone
				}
				return 0
			}
			if result.err != nil {
				events.write(controllerEvent{Type: "error", Message: result.err.Error()})
				continue
			}
			command := result.command
			switch command.Action {
			case "stop":
				pendingTarget = nil
				if moveCancel != nil {
					moveCancel()
				} else if err := desk.Stop(); err != nil {
					events.write(controllerEvent{Type: "error", Message: err.Error()})
				}
			case "set_position":
				cfg.Positions[command.Position] = command.Height
				if err := save(path, cfg); err != nil {
					events.write(controllerEvent{Type: "error", Message: err.Error()})
				} else {
					events.write(controllerEvent{Type: "positions", Positions: clonePositions(cfg.Positions)})
				}
			case "refresh":
				if !moving {
					height, speed, err := desk.HeightAndSpeed()
					if err != nil {
						events.write(controllerEvent{Type: "error", Message: err.Error()})
					} else {
						publishHeight(height, speed, movementStatus(speed))
					}
				}
			case "move", "adjust":
				target := command.Height
				if command.Position != "" {
					var found bool
					target, found = cfg.Positions[command.Position]
					if !found {
						events.write(controllerEvent{Type: "error", Message: fmt.Sprintf("position %q is not configured", command.Position)})
						continue
					}
				} else if command.Action == "adjust" {
					target = math.Float64frombits(lastHeight.Load()) + command.Delta
				}
				if _, err := EncodeHeight(target); err != nil {
					events.write(controllerEvent{Type: "error", Message: err.Error()})
					continue
				}
				if moving {
					pendingTarget = &target
					moveCancel()
				} else {
					startMove(target)
				}
			}
		case result := <-moveDone:
			moveCancel()
			moveCancel = nil
			moving = false
			if pendingTarget != nil {
				target := *pendingTarget
				pendingTarget = nil
				startMove(target)
				continue
			}
			if result.err != nil && !errors.Is(result.err, context.Canceled) {
				events.write(controllerEvent{Type: "error", Message: result.err.Error()})
			} else {
				events.write(controllerEvent{Type: "status", Status: "ready"})
			}
		case <-ticker.C:
			if moving {
				continue
			}
			height, speed, err := desk.HeightAndSpeed()
			if err != nil {
				events.write(controllerEvent{Type: "error", Message: err.Error()})
				continue
			}
			publishHeight(height, speed, movementStatus(speed))
		}
	}
}

func movementStatus(speed float64) string {
	if math.Abs(speed) >= .001 {
		return "moving"
	}
	return "ready"
}

func clonePositions(positions map[string]float64) map[string]float64 {
	cloned := make(map[string]float64, len(positions))
	for name, height := range positions {
		cloned[name] = height
	}
	return cloned
}
