package main

import (
	"context"
	"math"
	"strings"
	"testing"
)

func TestHeightEncodingRoundTrip(t *testing.T) {
	encoded, err := EncodeHeight(1.10)
	if err != nil {
		t.Fatal(err)
	}
	height, speed, err := DecodeHeight([]byte{encoded[0], encoded[1], 0x38, 0xff})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(height-1.10) > .0001 || math.Abs(speed-(-.02)) > .0001 {
		t.Fatalf("got height=%f speed=%f", height, speed)
	}
}

func TestHeightBounds(t *testing.T) {
	for _, height := range []float64{MinHeight - .01, MaxHeight + .01} {
		if _, err := EncodeHeight(height); err == nil {
			t.Fatalf("EncodeHeight(%v) succeeded", height)
		}
	}
}

func TestDecodeHeightRejectsWrongLength(t *testing.T) {
	if _, _, err := DecodeHeight([]byte{0, 0}); err == nil {
		t.Fatal("DecodeHeight accepted a short response")
	}
}

func TestConnectRejectsInvalidAddressBeforeUsingAdapter(t *testing.T) {
	_, err := Connect(context.Background(), "not-a-mac-address")
	if err == nil || !strings.Contains(err.Error(), "invalid Bluetooth address") {
		t.Fatalf("Connect returned %v", err)
	}
}
