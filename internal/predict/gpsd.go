package predict

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// Location represents a ground station position.
type Location struct {
	Lat float64 // degrees North
	Lon float64 // degrees East
	Alt float64 // meters above sea level
}

// tpvReport is the subset of a gpsd TPV JSON object we need.
type tpvReport struct {
	Class string  `json:"class"`
	Mode  int     `json:"mode"`
	Lat   float64 `json:"lat"`
	Lon   float64 `json:"lon"`
	Alt   float64 `json:"altMSL"`
}

// LocationFromGPSD connects to gpsd at the given host:port, sends a WATCH
// command, and reads TPV reports until a 2D or 3D fix is obtained.
func LocationFromGPSD(addr string, timeout time.Duration) (Location, error) {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return Location{}, fmt.Errorf("gpsd connect: %w", err)
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return Location{}, fmt.Errorf("gpsd set deadline: %w", err)
	}

	if _, err := fmt.Fprint(conn, `?WATCH={"enable":true,"json":true};`); err != nil {
		return Location{}, fmt.Errorf("gpsd watch: %w", err)
	}

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		var report tpvReport
		if err := json.Unmarshal(scanner.Bytes(), &report); err != nil {
			continue
		}
		if report.Class != "TPV" {
			continue
		}
		if report.Mode >= 2 {
			return Location{
				Lat: report.Lat,
				Lon: report.Lon,
				Alt: report.Alt,
			}, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return Location{}, fmt.Errorf("gpsd read: %w", err)
	}

	return Location{}, fmt.Errorf("gpsd: no fix obtained within %v", timeout)
}
