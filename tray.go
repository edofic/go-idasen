package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/slytomcat/systray"
)

type trayController struct {
	ctx        context.Context
	cancel     context.CancelFunc
	mac        string
	cfg        Config
	heightItem *systray.MenuItem
	stderr     io.Writer
	op         sync.Mutex
}

func runTray(mac string, cfg Config, stderr io.Writer) int {
	if err := PrepareBluetooth(); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	ctx, cancel := signalContext()
	controller := &trayController{ctx: ctx, cancel: cancel, mac: mac, cfg: cfg, stderr: stderr}
	systray.SetID("idasen")
	systray.Run(controller.ready, controller.cancel)
	return 0
}

func (t *trayController) ready() {
	systray.SetIcon(deskIcon())
	systray.SetTitle("IDÅSEN Desk")
	systray.SetTooltip("IDÅSEN Desk")

	t.heightItem = systray.AddMenuItem("Height: connecting…", "Current desk height")
	t.heightItem.Disable()
	systray.AddSeparator()

	names := make([]string, 0, len(t.cfg.Positions))
	for name := range t.cfg.Positions {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		name, target := name, t.cfg.Positions[name]
		label := fmt.Sprintf("%s  ·  %.0f cm", title(name), target*100)
		item := systray.AddMenuItem(label, fmt.Sprintf("Move desk to %.3f m", target))
		go func() {
			for range item.ClickedCh {
				t.move(name, target)
			}
		}()
	}

	systray.AddSeparator()
	refresh := systray.AddMenuItem("Refresh height", "Read the current desk height")
	quit := systray.AddMenuItem("Quit", "Close the IDÅSEN tray controller")
	go func() {
		for {
			select {
			case <-t.ctx.Done():
				systray.Quit()
				return
			case <-refresh.ClickedCh:
				t.refresh()
			case <-quit.ClickedCh:
				t.cancel()
			}
		}
	}()
	t.refresh()
}

func (t *trayController) refresh() {
	go t.run("Height: connecting…", func(d *Desk) error {
		height, _, err := d.HeightAndSpeed()
		if err == nil {
			t.setHeight(height)
		}
		return err
	})
}

func (t *trayController) move(name string, target float64) {
	go t.run("Moving to "+title(name)+"…", func(d *Desk) error {
		ctx, cancel := context.WithTimeout(t.ctx, 2*time.Minute)
		defer cancel()
		if err := d.MoveTo(ctx, target); err != nil {
			return err
		}
		height, _, err := d.HeightAndSpeed()
		if err == nil {
			t.setHeight(height)
		}
		return err
	})
}

func (t *trayController) run(status string, operation func(*Desk) error) {
	if !t.op.TryLock() {
		return
	}
	defer t.op.Unlock()
	t.heightItem.SetTitle(status)

	ctx, cancel := context.WithTimeout(t.ctx, 15*time.Second)
	desk, err := Connect(ctx, t.mac)
	cancel()
	if err == nil {
		err = operation(desk)
		_ = desk.Close()
	}
	if err != nil && t.ctx.Err() == nil {
		t.heightItem.SetTitle("Desk unavailable")
		systray.SetTooltip("IDÅSEN Desk · unavailable")
		fmt.Fprintln(t.stderr, err)
	}
}

func (t *trayController) setHeight(height float64) {
	label := fmt.Sprintf("Height: %.1f cm", height*100)
	t.heightItem.SetTitle(label)
	systray.SetTooltip("IDÅSEN Desk · " + strings.TrimPrefix(label, "Height: "))
}

func title(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	return strings.ToUpper(string(runes[0])) + string(runes[1:])
}

func deskIcon() []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	white := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	fill := func(x0, y0, x1, y1 int) {
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				img.SetNRGBA(x, y, white)
			}
		}
	}
	fill(3, 7, 29, 11)
	fill(6, 11, 9, 26)
	fill(23, 11, 26, 26)
	fill(3, 25, 12, 28)
	fill(20, 25, 29, 28)

	var data bytes.Buffer
	_ = png.Encode(&data, img)
	return data.Bytes()
}
