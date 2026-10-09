//go:build !tinygo

// Command dev is the local Sense inventory and BleRiot CLI.
package main

import (
	"github.com/burgrp/bleriot-sense/fw/v2/spec"
	"github.com/burgrp/bleriot/lib/shared/config"
	"github.com/burgrp/bleriot/lib/shared/inventory"
	"github.com/burgrp/bleriot/lib/site/cli"
)

var far = inventory.Channel{Name: "far", Number: 37, SpreadFactor: config.SpreadFactorS8}

var senseConfig = spec.Config{
	Mode:                       spec.ModeDS18B20,
	SampleIntervalMilliseconds: 1000,
	ADCSamples:                 16,
	DS18B20Sensors: []spec.DS18B20Sensor{
		{ID: [8]byte{40, 184, 35, 88, 4, 227, 61, 55}},
		{ID: [8]byte{40, 116, 244, 88, 4, 227, 61, 218}},
		{ID: [8]byte{40, 1, 182, 88, 4, 227, 61, 195}},
		{ID: [8]byte{40, 105, 64, 88, 4, 227, 61, 155}},
	},
}

func main() {
	// Mode must match the AR1/AR2/AR3/AC1 assembly population.
	cli.Start(inventory.Inventory{
		{
			Name:    "sense",
			Address: [4]byte{0xF7, 0x57, 0x17, 0x52},
			Key:     [16]byte{0xAD, 0xDD, 0xA8, 0xB4, 0x57, 0x07, 0x23, 0x61, 0x99, 0x20, 0x54, 0xDC, 0x5F, 0x6A, 0x95, 0xCB},
			Channel: far,
			Type:    spec.TypeForConfig(senseConfig),
			Config:  senseConfig,
		},
	})
}
