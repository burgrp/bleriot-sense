package main

func sampleChanged(previous int32, previousNull bool, current int32, currentNull bool, hysteresis uint32) bool {
	if previousNull != currentNull {
		return true
	}
	if currentNull {
		return false
	}
	if hysteresis <= 1 {
		return previous != current
	}

	delta := int64(current) - int64(previous)
	if delta < 0 {
		delta = -delta
	}
	return uint64(delta) >= uint64(hysteresis)
}
