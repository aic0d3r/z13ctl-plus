package cli

// cpu_power.go — AMD P-State controls exposed through cpufreq sysfs.

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/dahui/z13ctl/api"
)

// ReadCPUPower reads the current CPU frequency floor, EPP, and boost state.
func ReadCPUPower() (api.CPUPowerState, error) {
	return readCPUPower(cpuFreqDir)
}

func readCPUPower(base string) (api.CPUPowerState, error) {
	policies, err := cpuPolicies(base)
	if err != nil {
		return api.CPUPowerState{}, err
	}
	policy := policies[0]
	minFreq, err := readIntFile(policy + "/scaling_min_freq")
	if err != nil {
		return api.CPUPowerState{}, fmt.Errorf("reading CPU minimum frequency: %w", err)
	}
	minLimit, err := readIntFile(policy + "/cpuinfo_min_freq")
	if err != nil {
		return api.CPUPowerState{}, fmt.Errorf("reading CPU frequency lower limit: %w", err)
	}
	maxLimit, err := readIntFile(policy + "/cpuinfo_max_freq")
	if err != nil {
		return api.CPUPowerState{}, fmt.Errorf("reading CPU frequency upper limit: %w", err)
	}
	epp, err := ReadTrimmedFile(policy + "/energy_performance_preference")
	if err != nil {
		return api.CPUPowerState{}, fmt.Errorf("reading CPU EPP: %w", err)
	}
	choices, err := ReadTrimmedFile(policy + "/energy_performance_available_preferences")
	if err != nil {
		return api.CPUPowerState{}, fmt.Errorf("reading CPU EPP choices: %w", err)
	}
	boost, err := readIntFile(base + "/boost")
	if err != nil {
		return api.CPUPowerState{}, fmt.Errorf("reading CPU boost: %w", err)
	}
	return api.CPUPowerState{
		MinFrequencyKHz: minFreq,
		MinLimitKHz:     minLimit,
		MaxLimitKHz:     maxLimit,
		EPP:             epp,
		EPPChoices:      strings.Fields(choices),
		Boost:           boost != 0,
	}, nil
}

// SetCPUMinFrequency sets the minimum frequency on every CPU policy.
func SetCPUMinFrequency(khz int) error {
	return setCPUMinFrequency(cpuFreqDir, khz)
}

func setCPUMinFrequency(base string, khz int) error {
	policies, err := cpuPolicies(base)
	if err != nil {
		return err
	}
	minLimit, err := readIntFile(policies[0] + "/cpuinfo_min_freq")
	if err != nil {
		return fmt.Errorf("reading CPU frequency lower limit: %w", err)
	}
	maxLimit, err := readIntFile(policies[0] + "/cpuinfo_max_freq")
	if err != nil {
		return fmt.Errorf("reading CPU frequency upper limit: %w", err)
	}
	if khz < minLimit || khz > maxLimit {
		return fmt.Errorf("CPU minimum frequency must be between %d and %d kHz", minLimit, maxLimit)
	}
	return writePolicies(policies, "scaling_min_freq", strconv.Itoa(khz)+"\n")
}

// SetCPUEPP sets the AMD energy performance preference on every CPU policy.
func SetCPUEPP(value string) error {
	return setCPUEPP(cpuFreqDir, value)
}

func setCPUEPP(base, value string) error {
	policies, err := cpuPolicies(base)
	if err != nil {
		return err
	}
	choices, err := ReadTrimmedFile(policies[0] + "/energy_performance_available_preferences")
	if err != nil {
		return fmt.Errorf("reading CPU EPP choices: %w", err)
	}
	if !slices.Contains(strings.Fields(choices), value) {
		return fmt.Errorf("unsupported CPU EPP %q", value)
	}
	return writePolicies(policies, "energy_performance_preference", value+"\n")
}

// SetCPUBoost enables or disables CPU boost without changing GPU clocks.
func SetCPUBoost(enabled bool) error {
	value := "0\n"
	if enabled {
		value = "1\n"
	}
	if err := os.WriteFile(cpuFreqDir+"/boost", []byte(value), 0o644); err != nil {
		return fmt.Errorf("setting CPU boost: %w", err)
	}
	return nil
}

func cpuPolicies(base string) ([]string, error) {
	policies, err := filepath.Glob(base + "/policy*")
	if err != nil {
		return nil, fmt.Errorf("finding CPU policies: %w", err)
	}
	if len(policies) == 0 {
		return nil, fmt.Errorf("no CPU frequency policies found")
	}
	return policies, nil
}

// ReadTrimmedFile reads a file and trims surrounding whitespace. Shared by
// cmd/, internal/cli and internal/daemon (sysfs attribute reads).
func ReadTrimmedFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func writePolicies(policies []string, name, value string) error {
	previous := make([]string, len(policies))
	for i, policy := range policies {
		path := policy + "/" + name
		old, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}
		previous[i] = string(old)
	}
	for i, policy := range policies {
		path := policy + "/" + name
		if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
			for j := 0; j < i; j++ {
				_ = os.WriteFile(policies[j]+"/"+name, []byte(previous[j]), 0o644)
			}
			return fmt.Errorf("writing %s: %w", path, err)
		}
	}
	return nil
}
