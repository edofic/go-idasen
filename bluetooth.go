package main

import (
	"sync"

	"tinygo.org/x/bluetooth"
)

var (
	systemAdapter = bluetooth.DefaultAdapter
	adapterOnce   sync.Once
	adapterErr    error
)

// PrepareBluetooth initializes the operating system's Bluetooth stack.
// On Linux, tinygo.org/x/bluetooth talks to BlueZ over D-Bus; it does not take
// raw ownership of the HCI adapter.
func PrepareBluetooth() error {
	adapterOnce.Do(func() {
		adapterErr = systemAdapter.Enable()
	})
	return adapterErr
}
