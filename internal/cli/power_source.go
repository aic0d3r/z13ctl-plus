package cli

// power_source.go - AC power detection through power_supply sysfs.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// PowerSourceAC indicates that at least one external supply is online.
	PowerSourceAC = "ac"
	// PowerSourceBattery indicates that external supplies are present but offline.
	PowerSourceBattery = "battery"
)

// ReadPowerSource reports whether the system is running on AC or battery.
func ReadPowerSource() (string, error) {
	return readPowerSource(powerSupplyDir)
}

func readPowerSource(root string) (string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}

	found := false
	for _, entry := range entries {
		base := filepath.Join(root, entry.Name())
		typeData, readErr := os.ReadFile(base + "/type")
		if readErr != nil || !isExternalSupply(strings.TrimSpace(string(typeData))) {
			continue
		}
		online, readErr := os.ReadFile(base + "/online")
		if readErr != nil {
			continue
		}
		found = true
		if strings.TrimSpace(string(online)) == "1" {
			return PowerSourceAC, nil
		}
	}
	if !found {
		return "", fmt.Errorf("no readable external power supply")
	}
	return PowerSourceBattery, nil
}

func isExternalSupply(supplyType string) bool {
	switch supplyType {
	case "Mains", "USB", "USB_C", "USB_PD", "Wireless":
		return true
	default:
		return false
	}
}
