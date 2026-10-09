//go:build tinygo

package sense

import (
	"device/py32"
	"machine"
	"runtime/interrupt"
	"runtime/volatile"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/burgrp/bleriot-sense/fw/v2/spec"
	"github.com/burgrp/bleriot/lib/node"
	"github.com/burgrp/bleriot/lib/node/pan211x"
	"github.com/burgrp/tinygo-drivers/onewire"
	"github.com/burgrp/tinygo-drivers/onewire/ds18b20"
)

const (
	pinSensor  = machine.PA0
	pinSpiSck  = machine.PA1
	pinSpiData = machine.PA2
	pinSpiCsn  = machine.PB0

	defaultSampleInterval    = time.Second
	defaultADCSamples        = 16
	maximumADCSamples        = 64
	maximumInt32             = uint64(1<<31 - 1)
	adc12BitMask             = uint32(1<<12 - 1)
	flashOptionTriggerOffset = uintptr(0x80)
	flashKey1                = uint32(0x45670123)
	flashKey2                = uint32(0xcdef89ab)
	flashOptionKey1          = uint32(0x08192a3b)
	flashOptionKey2          = uint32(0x4c5d6e7f)
	flashStatusClear         = py32.Flash_SR_EOP | py32.Flash_SR_WRPERR | py32.Flash_SR_OPTVERR
)

var (
	flowPulses      atomic.Int32
	oneWireHardware py32OneWireHardware
	oneWireBus      = onewire.NewMaster(&oneWireHardware)
	temperatureBus  = ds18b20.NewNetwork(&oneWireBus)
)

type ds18b20Phase uint8

const (
	ds18b20Configure ds18b20Phase = iota
	ds18b20StartConversion
	ds18b20WaitConversion
	ds18b20ReadSensors
)

type Device struct {
	mode                     spec.Mode
	sample                   int32
	pulseSnapshot            int32
	previousPulses           uint32
	ds18b20Samples           [spec.MaxDS18B20Sensors]int32
	ds18b20IDs               [spec.MaxDS18B20Sensors]ds18b20.ROM
	ds18b20NextAction        int64
	ds18b20ConversionStarted int64
	ds18b20Ready             uint32
	ds18b20Configured        uint32
	ds18b20Searcher          onewire.Searcher
	ds18b20DiscoveredFirst   ds18b20.ROM
	ds18b20DiscoveredCount   uint8
	ds18b20Count             uint8
	ds18b20Index             uint8
	ds18b20Phase             ds18b20Phase
	ds18b20Ambiguous         bool
	ready                    bool
}

// Run starts the Sense firmware with baked provisioning and configuration.
func Run(provisioning node.Provisioning, config spec.Config) {
	ensureCsnPinIsGPIO()
	config = normalizeConfig(config)

	device := &Device{mode: config.Mode}
	switch config.Mode {
	case spec.ModeNTC, spec.ModePressure:
		initADC()
		device.sample = readAverageADC(int(config.ADCSamples))
		device.ready = analogSampleValid(config.Mode, device.sample)
	case spec.ModeFlow:
		initFlowCounter()
	case spec.ModeDS18B20:
		initOneWire()
		device.initDS18B20(config)
	default:
		halt("unsupported sensor mode")
	}

	bleNode, err := pan211x.StartNode(provisioning, pinSpiSck, pinSpiData, pinSpiCsn, device)
	if err != nil {
		halt("failed to start BleRiot node: " + err.Error())
	}

	intervalNanoseconds := int64(config.SampleIntervalMilliseconds) * int64(time.Millisecond)
	nextSample := monotonicNanoseconds() + intervalNanoseconds
	for {
		bleNode.Poll()

		now := monotonicNanoseconds()
		if device.mode == spec.ModeDS18B20 {
			device.serviceDS18B20(now, intervalNanoseconds)
			continue
		}
		if now >= nextSample {
			device.acquire(config)
			nextSample += intervalNanoseconds
			if now >= nextSample {
				nextSample = now + intervalNanoseconds
			}
		}
	}
}

func (device *Device) Read(tag uint16) (value int32, null bool) {
	if device.mode == spec.ModeDS18B20 {
		for index := 0; index < int(device.ds18b20Count); index++ {
			if tag != spec.DS18B20RegisterTag(index) {
				continue
			}
			if device.ds18b20Ready&(uint32(1)<<index) == 0 {
				return 0, true
			}
			return device.ds18b20Samples[index], false
		}
		return 0, true
	}
	if !device.ready {
		return 0, true
	}

	switch tag {
	case spec.RegSample:
		return device.sample, false
	case spec.RegPulseCount:
		if device.mode == spec.ModeFlow {
			return device.pulseSnapshot, false
		}
	}
	return 0, true
}

func (device *Device) Write(tag uint16, value int32, null bool) {
}

func (device *Device) acquire(config spec.Config) {
	switch config.Mode {
	case spec.ModeNTC, spec.ModePressure:
		device.sample = readAverageADC(int(config.ADCSamples))
		device.ready = analogSampleValid(config.Mode, device.sample)
	case spec.ModeFlow:
		current := uint32(flowPulses.Load())
		delta := current - device.previousPulses
		device.previousPulses = current

		frequencyMilliHz := uint64(delta) * 1_000_000 / uint64(config.SampleIntervalMilliseconds)
		if frequencyMilliHz > maximumInt32 {
			frequencyMilliHz = maximumInt32
		}
		device.sample = int32(frequencyMilliHz)
		device.pulseSnapshot = int32(current)
		device.ready = true
	}
}

func (device *Device) serviceDS18B20(now, sampleIntervalNanoseconds int64) {
	if now < device.ds18b20NextAction {
		return
	}

	switch device.ds18b20Phase {
	case ds18b20Configure:
		if device.ds18b20Count == 1 && device.ds18b20IDs[0] == (ds18b20.ROM{}) {
			device.serviceDS18B20Discovery(sampleIntervalNanoseconds)
			return
		}
		if device.ds18b20Index >= device.ds18b20Count {
			device.ds18b20Index = 0
			device.ds18b20Phase = ds18b20StartConversion
			return
		}

		index := device.ds18b20Index
		device.ds18b20Index++
		mask := uint32(1) << index
		if device.ds18b20Configured&mask != 0 {
			return
		}
		if device.ds18b20IDs[index] == (ds18b20.ROM{}) {
			device.ds18b20Ready &^= mask
			return
		}
		status := temperatureBus.Configure12Bit(device.ds18b20IDs[index])
		if status == ds18b20.StatusOK {
			device.ds18b20Configured |= mask
		} else if status == ds18b20.StatusNoPresence || status == ds18b20.StatusBusStuckLow {
			device.ds18b20Ready = 0
			device.ds18b20Configured = 0
			device.finishDS18B20Cycle(sampleIntervalNanoseconds)
		} else {
			device.ds18b20Ready &^= mask
		}
		return

	case ds18b20StartConversion:
		if device.ds18b20Configured == 0 {
			device.finishDS18B20Cycle(sampleIntervalNanoseconds)
			return
		}
		if temperatureBus.RequireExternalPowerAll() != ds18b20.StatusOK ||
			temperatureBus.StartConversionAll() != ds18b20.StatusOK {
			device.ds18b20Ready = 0
			device.ds18b20Configured = 0
			device.finishDS18B20Cycle(sampleIntervalNanoseconds)
			return
		}
		device.ds18b20ConversionStarted = monotonicNanoseconds()
		device.ds18b20NextAction = device.ds18b20ConversionStarted + int64(ds18b20.ConversionWait12Bit)
		device.ds18b20Phase = ds18b20WaitConversion
		return

	case ds18b20WaitConversion:
		device.ds18b20Index = 0
		device.ds18b20Phase = ds18b20ReadSensors
		return

	case ds18b20ReadSensors:
		if device.ds18b20Index >= device.ds18b20Count {
			device.finishDS18B20Cycle(sampleIntervalNanoseconds)
			return
		}

		index := device.ds18b20Index
		device.ds18b20Index++
		mask := uint32(1) << index
		if device.ds18b20Configured&mask == 0 {
			device.ds18b20Ready &^= mask
			return
		}
		sample, status := temperatureBus.ReadTemperatureRaw(device.ds18b20IDs[index])
		if status == ds18b20.StatusOK {
			device.ds18b20Samples[index] = int32(sample)
			device.ds18b20Ready |= mask
		} else if status == ds18b20.StatusNoPresence || status == ds18b20.StatusBusStuckLow {
			device.ds18b20Ready = 0
			device.ds18b20Configured = 0
			device.finishDS18B20Cycle(sampleIntervalNanoseconds)
		} else {
			device.ds18b20Ready &^= mask
			device.ds18b20Configured &^= mask
		}
	}
}

func (device *Device) finishDS18B20Cycle(sampleIntervalNanoseconds int64) {
	current := monotonicNanoseconds()
	if device.ds18b20ConversionStarted == 0 {
		device.ds18b20NextAction = current + sampleIntervalNanoseconds
	} else {
		device.ds18b20NextAction = device.ds18b20ConversionStarted + sampleIntervalNanoseconds
	}
	if device.ds18b20NextAction < current {
		device.ds18b20NextAction = current
	}
	device.ds18b20ConversionStarted = 0
	device.ds18b20Index = 0
	device.ds18b20Phase = ds18b20Configure
}

func (device *Device) initDS18B20(config spec.Config) {
	count := len(config.DS18B20Sensors)
	if count == 0 {
		count = 1
	}
	if count > spec.MaxDS18B20Sensors {
		count = spec.MaxDS18B20Sensors
	}
	device.ds18b20Count = uint8(count)
	for index := 0; index < len(config.DS18B20Sensors) && index < count; index++ {
		device.ds18b20IDs[index] = ds18b20.ROM(config.DS18B20Sensors[index].ID)
	}

	first, discovered, complete := discoverDS18B20s()
	if count == 1 && device.ds18b20IDs[0] == (ds18b20.ROM{}) {
		if complete && discovered == 1 {
			device.ds18b20IDs[0] = first
		} else if complete && discovered > 1 {
			println("Multiple DS18B20 sensors found; configure an ID for the temperature register")
			device.ds18b20Ambiguous = true
		}
	}
}

func (device *Device) serviceDS18B20Discovery(sampleIntervalNanoseconds int64) {
	if device.ds18b20Ambiguous {
		device.finishDS18B20Cycle(sampleIntervalNanoseconds)
		return
	}

	rom, status := device.ds18b20Searcher.Next(&oneWireBus)
	switch status {
	case onewire.SearchFound:
		if rom[0] != ds18b20.FamilyCode {
			return
		}
		id := ds18b20.ROM(rom)
		printDS18B20ID(id)
		if device.ds18b20DiscoveredCount == 0 {
			device.ds18b20DiscoveredFirst = id
		}
		if device.ds18b20DiscoveredCount < 255 {
			device.ds18b20DiscoveredCount++
		}
		return

	case onewire.SearchDone:
		if device.ds18b20DiscoveredCount == 1 {
			device.ds18b20IDs[0] = device.ds18b20DiscoveredFirst
			device.ds18b20Searcher.Reset()
			device.ds18b20DiscoveredCount = 0
			device.ds18b20DiscoveredFirst = ds18b20.ROM{}
			return
		}
		if device.ds18b20DiscoveredCount > 1 {
			println("Multiple DS18B20 sensors found; configure an ID for the temperature register")
			device.ds18b20Ambiguous = true
		}
	default:
		println("1-Wire discovery failed with status", uint8(status))
	}

	device.ds18b20Searcher.Reset()
	device.ds18b20DiscoveredCount = 0
	device.ds18b20DiscoveredFirst = ds18b20.ROM{}
	device.ds18b20Ready = 0
	device.finishDS18B20Cycle(sampleIntervalNanoseconds)
}

func discoverDS18B20s() (first ds18b20.ROM, count uint8, complete bool) {
	var searcher onewire.Searcher
	for {
		rom, status := searcher.Next(&oneWireBus)
		switch status {
		case onewire.SearchFound:
			if rom[0] != ds18b20.FamilyCode {
				continue
			}
			id := ds18b20.ROM(rom)
			printDS18B20ID(id)
			if count == 0 {
				first = id
			}
			if count < 255 {
				count++
			}
		case onewire.SearchDone:
			if count == 0 {
				println("No DS18B20 sensors found")
			}
			return first, count, true
		default:
			println("1-Wire discovery failed with status", uint8(status))
			return first, count, false
		}
	}
}

func printDS18B20ID(id ds18b20.ROM) {
	print("DS18B20 ID: [8]byte{")
	for index, value := range id {
		if index > 0 {
			print(", ")
		}
		print(value)
	}
	println("}")
}

//go:linkname monotonicNanoseconds runtime.nanotime
func monotonicNanoseconds() int64

func normalizeConfig(config spec.Config) spec.Config {
	if config.SampleIntervalMilliseconds == 0 {
		config.SampleIntervalMilliseconds = uint32(defaultSampleInterval / time.Millisecond)
	}
	if config.ADCSamples == 0 {
		config.ADCSamples = defaultADCSamples
	}
	if config.ADCSamples > maximumADCSamples {
		config.ADCSamples = maximumADCSamples
	}
	minimumDS18B20Interval := uint32(ds18b20.ConversionWait12Bit / time.Millisecond)
	if config.Mode == spec.ModeDS18B20 && config.SampleIntervalMilliseconds < minimumDS18B20Interval {
		config.SampleIntervalMilliseconds = minimumDS18B20Interval
	}
	return config
}

func initADC() {
	pinSensor.Configure(machine.PinConfig{Mode: machine.PinInputAnalog})
	py32.RCC.APBENR2.SetBits(py32.RCC_APBENR2_ADCEN)
	calibrateADC()

	py32.ADC.CFGR2.Set(py32.ADC_CFGR2_CKMODE_1 << py32.ADC_CFGR2_CKMODE_Pos)
	py32.ADC.CFGR1.Set(py32.ADC_CFGR1_OVRMOD)
	py32.ADC.SMPR.Set(py32.ADC_SMPR_SMP_Msk)
	py32.ADC.CHSELR.Set(py32.ADC_CHSELR_CHSEL0)
}

func calibrateADC() {
	var results [5]int32
	for index := range results {
		py32.ADC.CR.SetBits(py32.ADC_CR_ADCAL)
		for py32.ADC.CR.HasBits(py32.ADC_CR_ADCAL) {
		}
		results[index] = int32(py32.ADC.CALRR1.Get() << 9)
	}
	for index := 0; index < len(results); index++ {
		minimum := index
		for candidate := index + 1; candidate < len(results); candidate++ {
			if results[candidate] < results[minimum] {
				minimum = candidate
			}
		}
		results[index], results[minimum] = results[minimum], results[index]
	}
	py32.ADC.CALFIR1.Set(uint32(results[2] >> 9))
	py32.ADC.CALFIR2.Set(py32.ADC.CALRR2.Get())
	py32.ADC.CCSR.SetBits(py32.ADC_CCSR_CALSET)
	time.Sleep(time.Millisecond)
}

func readAverageADC(samples int) int32 {
	var total uint32
	for range samples {
		py32.ADC.CR.SetBits(py32.ADC_CR_ADEN)
		time.Sleep(time.Microsecond)
		py32.ADC.ISR.Set(py32.ADC_ISR_EOC | py32.ADC_ISR_EOSEQ | py32.ADC_ISR_OVR)
		py32.ADC.CR.SetBits(py32.ADC_CR_ADSTART)
		for py32.ADC.ISR.Get()&(py32.ADC_ISR_EOC|py32.ADC_ISR_EOSEQ) == 0 {
		}
		total += py32.ADC.DR.Get() & adc12BitMask
	}
	return int32((total + uint32(samples/2)) / uint32(samples))
}

func initFlowCounter() {
	pinSensor.Configure(machine.PinConfig{Mode: machine.PinInput})

	py32.EXTI.EXTICR1.ClearBits(py32.EXTI_EXTICR1_EXTI0_Msk)
	py32.EXTI.PR.Set(py32.EXTI_PR_PR0)
	py32.EXTI.RTSR.SetBits(py32.EXTI_RTSR_RT0)
	py32.EXTI.FTSR.ClearBits(py32.EXTI_FTSR_FT0)
	py32.EXTI.IMR.SetBits(py32.EXTI_IMR_IM0)

	flowInterrupt := interrupt.New(py32.IRQ_EXTI0_1, handleFlowInterrupt)
	flowInterrupt.SetPriority(0)
	flowInterrupt.Enable()
}

func handleFlowInterrupt(interrupt.Interrupt) {
	if !py32.EXTI.PR.HasBits(py32.EXTI_PR_PR0) {
		return
	}
	py32.EXTI.PR.Set(py32.EXTI_PR_PR0)
	flowPulses.Add(1)
}

func ensureCsnPinIsGPIO() {
	if py32.FLASH.OPTR.HasBits(py32.Flash_OPTR_NRST_MODE) {
		return
	}

	println("Enabling PB0 GPIO option for PAN2110 CSN")
	for py32.FLASH.SR.HasBits(py32.Flash_SR_BSY) {
	}
	if py32.FLASH.CR.HasBits(py32.Flash_CR_LOCK) {
		py32.FLASH.KEYR.Set(flashKey1)
		py32.FLASH.KEYR.Set(flashKey2)
	}
	if py32.FLASH.CR.HasBits(py32.Flash_CR_OPTLOCK) {
		py32.FLASH.OPTKEYR.Set(flashOptionKey1)
		py32.FLASH.OPTKEYR.Set(flashOptionKey2)
	}
	if py32.FLASH.CR.Get()&(py32.Flash_CR_LOCK|py32.Flash_CR_OPTLOCK) != 0 {
		halt("failed to unlock option bytes")
	}

	py32.FLASH.SR.Set(flashStatusClear)
	py32.FLASH.OPTR.SetBits(py32.Flash_OPTR_NRST_MODE)
	py32.FLASH.CR.SetBits(py32.Flash_CR_OPTSTRT)
	triggerFlashOptionProgramming()
	for py32.FLASH.SR.HasBits(py32.Flash_SR_BSY) {
	}
	if py32.FLASH.SR.Get()&(py32.Flash_SR_WRPERR|py32.Flash_SR_OPTVERR) != 0 {
		halt("failed to program PB0 GPIO option")
	}
	py32.FLASH.CR.SetBits(py32.Flash_CR_OBL_LAUNCH)
	halt("option-byte reload did not reset")
}

func triggerFlashOptionProgramming() {
	// Puya requires a write to this undocumented trigger register after OPTSTRT.
	// It is absent from the SVD, so derive it from TinyGo's FLASH declaration.
	address := uintptr(unsafe.Pointer(py32.FLASH)) + flashOptionTriggerOffset
	(*volatile.Register32)(unsafe.Pointer(address)).Set(0xff)
}

func halt(message string) {
	println(message)
	for {
		time.Sleep(time.Second)
	}
}
