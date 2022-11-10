package config

import (
	"log"
	"strconv"
)

const (
	//DefaultTicker is a ticker for periodic scaling, the unit is seconds, the default is 30 seconds
	DefaultTicker = 5

	//TickerLabel can change the ticker setting
	TickerLabel = "com.openfaas.scale.ticker"
)

// ExtractLabelValue will parse the provided raw label value and if it fails
// it will return the provided fallback value and log an message
// Copy from faas/gateway private function extractLabelValue
func ExtractLabelValue(rawLabelValue string, fallback int) int {
	if len(rawLabelValue) <= 0 {
		return fallback
	}

	value, err := strconv.Atoi(rawLabelValue)

	if err != nil {
		log.Printf("Provided label value %s should be of type uint", rawLabelValue)
		return fallback
	}

	return value
}
