// Package capture records NOAA APT satellite passes to WAV files, either
// from a real RTL-SDR dongle or via synthetic tone generation for testing.
package capture

import "strings"

// Satellite describes a NOAA APT bird: its common name, NORAD catalog
// number, and downlink frequency in hertz.
type Satellite struct {
	Name    string
	NoradID int
	Freq    int // downlink frequency in Hz
}

// Satellites is the catalog of active NOAA APT satellites. All three
// transmit on frequencies in the 137 MHz VHF band.
var Satellites = []Satellite{
	{Name: "NOAA-15", NoradID: 25338, Freq: 137620000},
	{Name: "NOAA-18", NoradID: 28654, Freq: 137912500},
	{Name: "NOAA-19", NoradID: 33591, Freq: 137100000},
}

// SatelliteByNoradID returns the satellite with the given NORAD catalog ID,
// or nil if not found.
func SatelliteByNoradID(id int) *Satellite {
	for i := range Satellites {
		if Satellites[i].NoradID == id {
			return &Satellites[i]
		}
	}
	return nil
}

// SatelliteByName returns the satellite with the given name (case-insensitive),
// or nil if not found.
func SatelliteByName(name string) *Satellite {
	upper := strings.ToUpper(name)
	for i := range Satellites {
		if strings.ToUpper(Satellites[i].Name) == upper {
			return &Satellites[i]
		}
	}
	return nil
}
