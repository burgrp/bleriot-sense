//go:build tinygo

package main

import (
	"device/py32"
	"machine"
	"runtime/interrupt"
	"runtime/volatile"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/burgrp/bleriot-sense/fw-003/spec"
	"github.com/burgrp/bleriot/lib/node"
	"github.com/burgrp/bleriot/lib/node/pan211x"
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

var flowPulses atomic.Int32

type Device struct {
	mode           spec.Mode
	sample         int32
	pulseSnapshot  int32
	previousPulses uint32
	ready          bool
}

func bleriotMain(provisioning node.Provisioning, config spec.Config) {
	ensureCsnPinIsGPIO()
	config = normalizeConfig(config)

	device := &Device{mode: config.Mode}
	switch config.Mode {
	case spec.ModeNTC, spec.ModePressure:
		initADC()
		device.sample = readAverageADC(int(config.ADCSamples))
		device.ready = true
	case spec.ModeFlow:
		initFlowCounter()
	default:
		halt("unsupported sensor mode")
	}

	bleNode, err := pan211x.StartNode(provisioning, pinSpiSck, pinSpiData, pinSpiCsn, device)
	if err != nil {
		halt("failed to start BleRiot node: " + err.Error())
	}

	lastSample, lastSampleNull := device.Read(spec.RegSample)
	lastPulses, lastPulsesNull := device.Read(spec.RegPulseCount)
	intervalNanoseconds := int64(config.SampleIntervalMilliseconds) * int64(time.Millisecond)
	nextSample := monotonicNanoseconds() + intervalNanoseconds
	for {
		bleNode.Poll()

		now := monotonicNanoseconds()
		if now >= nextSample {
			device.acquire(config)
			nextSample += intervalNanoseconds
			if now >= nextSample {
				nextSample = now + intervalNanoseconds
			}
		}

		sample, sampleNull := device.Read(spec.RegSample)
		if sampleChanged(lastSample, lastSampleNull, sample, sampleNull, config.SampleHysteresis) {
			lastSample, lastSampleNull = sample, sampleNull
			bleNode.Notify(spec.RegSample, sample, sampleNull)
		}

		if config.Mode == spec.ModeFlow {
			pulses, pulsesNull := device.Read(spec.RegPulseCount)
			if pulses != lastPulses || pulsesNull != lastPulsesNull {
				lastPulses, lastPulsesNull = pulses, pulsesNull
				bleNode.Notify(spec.RegPulseCount, pulses, pulsesNull)
			}
		}
	}
}

func (device *Device) Read(tag uint16) (value int32, null bool) {
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
