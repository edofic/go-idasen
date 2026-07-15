package main

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"time"

	"tinygo.org/x/bluetooth"
)

const (
	MinHeight = 0.62
	MaxHeight = 1.27

	heightUUID     = "99fa0021-338a-1024-8a49-009c0215f78a"
	commandUUID    = "99fa0002-338a-1024-8a49-009c0215f78a"
	referenceUUID  = "99fa0031-338a-1024-8a49-009c0215f78a"
	advertisedUUID = "99fa0001-338a-1024-8a49-009c0215f78a"
	dpgUUID        = "99fa0011-338a-1024-8a49-009c0215f78a"
)

var (
	cmdUp     = []byte{0x47, 0x00}
	cmdDown   = []byte{0x46, 0x00}
	cmdStop   = []byte{0xff, 0x00}
	cmdWakeup = []byte{0xfe, 0x00}
)

// Desk is a connected IDÅSEN desk.
type Desk struct {
	device bluetooth.Device
	chars  map[string]bluetooth.DeviceCharacteristic
}

// Connect dials a desk, discovers its GATT characteristics and wakes it up.
func Connect(ctx context.Context, address string) (*Desk, error) {
	mac, err := bluetooth.ParseMAC(address)
	if err != nil {
		return nil, fmt.Errorf("invalid Bluetooth address %q: %w", address, err)
	}

	type connectResult struct {
		device bluetooth.Device
		err    error
	}
	result := make(chan connectResult, 1)
	go func() {
		device, err := systemAdapter.Connect(
			bluetooth.Address{MACAddress: bluetooth.MACAddress{MAC: mac}},
			bluetooth.ConnectionParams{},
		)
		result <- connectResult{device: device, err: err}
	}()

	var device bluetooth.Device
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("connect to %s: %w", address, ctx.Err())
	case connected := <-result:
		if connected.err != nil {
			return nil, fmt.Errorf("connect to %s: %w", address, connected.err)
		}
		device = connected.device
	}

	services, err := device.DiscoverServices(nil)
	if err != nil {
		_ = device.Disconnect()
		return nil, fmt.Errorf("discover services: %w", err)
	}
	d := &Desk{device: device, chars: make(map[string]bluetooth.DeviceCharacteristic)}
	for _, service := range services {
		chars, err := service.DiscoverCharacteristics(nil)
		if err != nil {
			d.Close()
			return nil, fmt.Errorf("discover characteristics for %s: %w", service.UUID(), err)
		}
		for _, characteristic := range chars {
			d.chars[characteristic.UUID().String()] = characteristic
		}
	}
	if err := d.Wakeup(); err != nil {
		d.Close()
		return nil, err
	}
	return d, nil
}

func (d *Desk) Close() error { return d.device.Disconnect() }

func (d *Desk) characteristic(uuid string) (bluetooth.DeviceCharacteristic, error) {
	parsed, err := bluetooth.ParseUUID(uuid)
	if err != nil {
		return bluetooth.DeviceCharacteristic{}, fmt.Errorf("invalid characteristic UUID %q: %w", uuid, err)
	}
	c, ok := d.chars[parsed.String()]
	if !ok {
		return bluetooth.DeviceCharacteristic{}, fmt.Errorf("desk does not expose characteristic %s", uuid)
	}
	return c, nil
}

func (d *Desk) write(uuid string, value []byte, noResponse bool) error {
	c, err := d.characteristic(uuid)
	if err != nil {
		return err
	}
	if noResponse {
		_, err = c.WriteWithoutResponse(value)
	} else {
		_, err = c.Write(value)
	}
	return err
}

// Wakeup supports both original IDÅSEN and Linak DPG1C controllers.
func (d *Desk) Wakeup() error {
	if err := d.write(dpgUUID, []byte{0x7f, 0x86, 0x00}, false); err != nil {
		return err
	}
	if err := d.write(dpgUUID, []byte{0x7f, 0x86, 0x80, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17}, false); err != nil {
		return err
	}
	return d.write(commandUUID, cmdWakeup, false)
}

func (d *Desk) MoveUp() error   { return d.write(commandUUID, cmdUp, true) }
func (d *Desk) MoveDown() error { return d.write(commandUUID, cmdDown, true) }
func (d *Desk) Stop() error {
	if err := d.write(commandUUID, cmdStop, true); err != nil {
		return err
	}
	return d.write(referenceUUID, []byte{1, 0x80}, true)
}

// HeightAndSpeed reads the desk's current height in metres and speed in m/s.
func (d *Desk) HeightAndSpeed() (float64, float64, error) {
	c, err := d.characteristic(heightUUID)
	if err != nil {
		return 0, 0, err
	}
	raw := make([]byte, 512)
	n, err := c.Read(raw)
	if err != nil {
		return 0, 0, err
	}
	if n > len(raw) {
		return 0, 0, fmt.Errorf("height response is too large: %d bytes", n)
	}
	return DecodeHeight(raw[:n])
}

func DecodeHeight(raw []byte) (float64, float64, error) {
	if len(raw) != 4 {
		return 0, 0, fmt.Errorf("invalid height response: got %d bytes, want 4", len(raw))
	}
	height := float64(binary.LittleEndian.Uint16(raw[:2]))/10000 + MinHeight
	speed := float64(int16(binary.LittleEndian.Uint16(raw[2:]))) / 10000
	return height, speed, nil
}

func EncodeHeight(height float64) ([]byte, error) {
	if height < MinHeight || height > MaxHeight {
		return nil, fmt.Errorf("target %.3f m is outside %.3f–%.3f m", height, MinHeight, MaxHeight)
	}
	b := make([]byte, 2)
	binary.LittleEndian.PutUint16(b, uint16((height-MinHeight)*10000))
	return b, nil
}

// MoveTo repeatedly writes the reference input until the desk arrives or stalls.
func (d *Desk) MoveTo(ctx context.Context, target float64) error {
	return d.moveTo(ctx, target, nil)
}

// moveTo is MoveTo with an optional callback for live movement updates.
func (d *Desk) moveTo(ctx context.Context, target float64, update func(float64, float64)) error {
	data, err := EncodeHeight(target)
	if err != nil {
		return err
	}
	height, speed, err := d.HeightAndSpeed()
	if err != nil {
		return err
	}
	if update != nil {
		update(height, speed)
	}
	if math.Abs(height-target) < .005 {
		return nil
	}
	if err := d.write(commandUUID, cmdWakeup, false); err != nil {
		return err
	}
	if err := d.write(commandUUID, cmdStop, false); err != nil {
		return err
	}
	defer d.Stop()
	zeros, stalls := 0, 0
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		if err := d.write(referenceUUID, data, false); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
		height, speed, err = d.HeightAndSpeed()
		if err != nil {
			return err
		}
		if update != nil {
			update(height, speed)
		}
		if speed == 0 {
			zeros++
		} else {
			zeros, stalls = 0, 0
		}
		if zeros >= 2 {
			if math.Abs(height-target) < .005 {
				return nil
			}
			stalls++
			if stalls >= 3 {
				return errors.New("desk stopped before reaching target")
			}
			zeros = 0
		}
	}
}

// Monitor invokes callback whenever height or speed changes materially. It blocks until ctx is cancelled.
func (d *Desk) Monitor(ctx context.Context, callback func(float64, float64)) error {
	c, err := d.characteristic(heightUUID)
	if err != nil {
		return err
	}
	var previousHeight, previousSpeed float64
	err = c.EnableNotifications(func(raw []byte) {
		h, s, e := DecodeHeight(raw)
		if e != nil {
			return
		}
		if math.Abs(h-previousHeight) < .001 && math.Abs(s-previousSpeed) < .001 {
			return
		}
		previousHeight, previousSpeed = h, s
		callback(h, s)
	})
	if err != nil {
		return err
	}
	defer c.EnableNotifications(nil)
	<-ctx.Done()
	return ctx.Err()
}

// Discover returns the address of an advertising IDÅSEN desk, if one is found.
func Discover(ctx context.Context) (string, error) {
	uuid, err := bluetooth.ParseUUID(advertisedUUID)
	if err != nil {
		return "", err
	}
	var address string
	scanDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = systemAdapter.StopScan()
		case <-scanDone:
		}
	}()
	err = systemAdapter.Scan(func(adapter *bluetooth.Adapter, result bluetooth.ScanResult) {
		if result.AdvertisementPayload.HasServiceUUID(uuid) {
			address = result.Address.String()
			_ = adapter.StopScan()
		}
	})
	close(scanDone)
	if address != "" {
		return address, nil
	}
	if ctx.Err() != nil {
		return "", nil
	}
	return "", err
}
