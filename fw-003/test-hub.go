//go:build !tinygo

package main

import (
	"github.com/burgrp/bleriot-sense/fw-003/v2/spec"
	"github.com/burgrp/bleriot/lib/shared/config"
	"github.com/burgrp/bleriot/lib/shared/inventory"
	"github.com/burgrp/bleriot/lib/site/cli"
)

var (
	far = inventory.Channel{Name: "far", Number: 37, SpreadFactor: config.SpreadFactorS8}
)

func main() {
	// Mode must match the AR1/AR2/AR3/AC1 assembly population.
	const sensorMode = spec.ModeNTC

	cli.Start(inventory.Inventory{
		{
			Name:    "sense",
			Address: [4]byte{0xF7, 0x57, 0x17, 0x52},
			Key:     [16]byte{0xAD, 0xDD, 0xA8, 0xB4, 0x57, 0x07, 0x23, 0x61, 0x99, 0x20, 0x54, 0xDC, 0x5F, 0x6A, 0x95, 0xCB},
			Channel: far,
			Type:    spec.Type(sensorMode),
			Config: spec.Config{
				Mode:                       sensorMode,
				SampleIntervalMilliseconds: 1000,
				ADCSamples:                 16,
			},
		},
	})
}
