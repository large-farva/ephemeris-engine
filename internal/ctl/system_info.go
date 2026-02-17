package ctl

import (
	"fmt"
	"strings"
)

// SystemInfo shows runtime and hardware information from the daemon.
func SystemInfo(baseURL string, jsonOutput bool) error {
	baseURL = strings.TrimRight(baseURL, "/")

	var resp struct {
		GoVersion    string `json:"go_version"`
		OS           string `json:"os"`
		Arch         string `json:"arch"`
		DataRoot     string `json:"data_root"`
		ConfigDir    string `json:"config_dir"`
		SDRAvailable bool   `json:"sdr_available"`
		Disk         *struct {
			TotalBytes     uint64 `json:"total_bytes"`
			UsedBytes      uint64 `json:"used_bytes"`
			AvailableBytes uint64 `json:"available_bytes"`
		} `json:"disk"`
	}
	if err := getJSON(baseURL, "/api/system", &resp); err != nil {
		return err
	}

	if jsonOutput {
		return printJSON(resp)
	}

	fmt.Println()
	fmt.Println(header("  SYSTEM INFO"))
	fmt.Println("  " + strings.Repeat("─", 50))
	fmt.Printf("  Go version:  %s\n", resp.GoVersion)
	fmt.Printf("  OS/Arch:     %s/%s\n", resp.OS, resp.Arch)
	fmt.Printf("  Data root:   %s\n", resp.DataRoot)
	fmt.Printf("  Config dir:  %s\n", resp.ConfigDir)

	if resp.SDRAvailable {
		fmt.Printf("  SDR:         %s (rtl_fm found)\n", colorize(green, "AVAILABLE"))
	} else {
		fmt.Printf("  SDR:         %s (rtl_fm not found)\n", colorize(yellow, "NOT FOUND"))
	}

	if resp.Disk != nil {
		fmt.Printf("  Disk total:  %s\n", formatBytes(int64(resp.Disk.TotalBytes)))
		fmt.Printf("  Disk used:   %s\n", formatBytes(int64(resp.Disk.UsedBytes)))
		fmt.Printf("  Disk avail:  %s\n", formatBytes(int64(resp.Disk.AvailableBytes)))
	}

	fmt.Println()
	return nil
}
