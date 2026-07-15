package main

import (
	"strings"
	"testing"
)

func TestReadControllerCommands(t *testing.T) {
	results := readControllerCommands(strings.NewReader(
		"{\"action\":\"move\",\"position\":\"sit\"}\n" +
			"{\"action\":\"adjust\",\"delta\":0.005}\n" +
			"{\"action\":\"set_position\",\"position\":\"stand\",\"height\":1.1}\n",
	))
	for i := 0; i < 3; i++ {
		if result := <-results; result.err != nil {
			t.Fatalf("command %d: %v", i, result.err)
		}
	}
	if _, ok := <-results; ok {
		t.Fatal("command channel was not closed")
	}
}

func TestValidateControllerCommand(t *testing.T) {
	tests := []struct {
		name    string
		command controllerCommand
		wantErr bool
	}{
		{name: "move position", command: controllerCommand{Action: "move", Position: "sit"}},
		{name: "move height", command: controllerCommand{Action: "move", Height: .75}},
		{name: "move too low", command: controllerCommand{Action: "move", Height: .50}, wantErr: true},
		{name: "small adjustment", command: controllerCommand{Action: "adjust", Delta: -.005}},
		{name: "zero adjustment", command: controllerCommand{Action: "adjust"}, wantErr: true},
		{name: "save stand", command: controllerCommand{Action: "set_position", Position: "stand", Height: 1.10}},
		{name: "reserved position", command: controllerCommand{Action: "set_position", Position: "move", Height: 1.10}, wantErr: true},
		{name: "unknown", command: controllerCommand{Action: "launch"}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateControllerCommand(test.command)
			if (err != nil) != test.wantErr {
				t.Fatalf("got error %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestMovementStatus(t *testing.T) {
	if got := movementStatus(0); got != "ready" {
		t.Fatalf("got %q", got)
	}
	if got := movementStatus(.02); got != "moving" {
		t.Fatalf("got %q", got)
	}
}

func TestClonePositions(t *testing.T) {
	original := map[string]float64{"sit": .75}
	cloned := clonePositions(original)
	cloned["sit"] = .80
	if original["sit"] != .75 {
		t.Fatal("clone aliases original map")
	}
}
