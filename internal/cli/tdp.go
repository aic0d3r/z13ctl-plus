package cli

// tdp.go — PPT sysfs path discovery and I/O helpers for ASUS TDP control.
// Uses the asus-nb-wmi platform device attributes (NOT firmware-attributes,
// which have empty calibration data on the 2025 Z13).

import (
	"fmt"

	"github.com/dahui/z13ctl/api"
)

// TDP safety limits in watts, derived from G-Helper's model config for the
// 2025 ROG Flow Z13 (GZ302E) and Armoury Crate custom mode limits.
const (
	TDPMin       = 5  // absolute minimum
	TDPMaxSafe   = 75 // max in Armoury Crate custom mode
	TDPMaxForced = 93 // absolute max for GZ302E (G-Helper)
)

// pptBasePath is a variable only so UseSysfsRoots can redirect it in tests.
var pptBasePath = "/sys/devices/platform/asus-nb-wmi"

// StockProfilePPT maps stock platform_profile names to their actual PPT values
// as measured with ryzenadj on the 2025 Z13. The kernel's sysfs PPT attributes
// are a stale cache (initialized to 5W on module load) and do not reflect the
// EC's actual per-profile limits unless explicitly written.
var StockProfilePPT = map[string]api.TDPState{
	"quiet":       {PL1SPL: 40, PL2SPPT: 55, FPPT: 55, APUSPPT: 70, PlatformSPPT: 70},
	"balanced":    {PL1SPL: 52, PL2SPPT: 71, FPPT: 70, APUSPPT: 70, PlatformSPPT: 70},
	"performance": {PL1SPL: 70, PL2SPPT: 86, FPPT: 86, APUSPPT: 70, PlatformSPPT: 70},
}

// ReadEffectivePPT returns the current PPT values. If sysfs returns the stale
// kernel cache (PL1 == 5) and the active profile is a known stock profile,
// the measured per-profile defaults are returned instead.
func ReadEffectivePPT(profile string) (api.TDPState, error) {
	s, err := ReadAllPPT()
	if err != nil {
		return s, err
	}
	if s.PL1SPL == TDPMin {
		if stock, ok := StockProfilePPT[profile]; ok {
			return stock, nil
		}
	}
	return s, nil
}

// ReadAllPPT reads all 5 PPT values and returns a TDPState.
func ReadAllPPT() (api.TDPState, error) {
	var s api.TDPState
	var err error
	if s.PL1SPL, err = readIntFile(pptBasePath + "/ppt_pl1_spl"); err != nil {
		return s, fmt.Errorf("reading ppt_pl1_spl: %w", err)
	}
	if s.PL2SPPT, err = readIntFile(pptBasePath + "/ppt_pl2_sppt"); err != nil {
		return s, fmt.Errorf("reading ppt_pl2_sppt: %w", err)
	}
	if s.FPPT, err = readIntFile(pptBasePath + "/ppt_fppt"); err != nil {
		return s, fmt.Errorf("reading ppt_fppt: %w", err)
	}
	if s.APUSPPT, err = readIntFile(pptBasePath + "/ppt_apu_sppt"); err != nil {
		return s, fmt.Errorf("reading ppt_apu_sppt: %w", err)
	}
	if s.PlatformSPPT, err = readIntFile(pptBasePath + "/ppt_platform_sppt"); err != nil {
		return s, fmt.Errorf("reading ppt_platform_sppt: %w", err)
	}
	return s, nil
}

// ValidateTDP enforces the shared TDP safety rule, used by both the CLI and
// the daemon: PL1 (sustained) is capped at TDPMaxSafe unless force allows up
// to TDPMaxForced; PL2/PL3 (burst) may use the hardware max without force.
func ValidateTDP(pl1, pl2, pl3 int, force bool) error {
	pl1Max := TDPMaxSafe
	if force {
		pl1Max = TDPMaxForced
	}
	if pl1 < TDPMin || pl1 > pl1Max {
		if pl1 > TDPMaxSafe && !force {
			return fmt.Errorf("PL1 %dW exceeds safe sustained max (%dW); use force to allow up to %dW", pl1, TDPMaxSafe, TDPMaxForced)
		}
		return fmt.Errorf("PL1 %dW out of range %d–%d", pl1, TDPMin, pl1Max)
	}
	for _, v := range []int{pl2, pl3} {
		if v < TDPMin || v > TDPMaxForced {
			return fmt.Errorf("TDP %dW out of range %d–%d", v, TDPMin, TDPMaxForced)
		}
	}
	return nil
}

// SetTDP writes all PPT values. pl1/pl2/pl3 override the unified watts value
// when non-zero. APU sPPT and Platform sPPT always follow PL2.
func SetTDP(watts, pl1, pl2, pl3 int) error {
	if pl1 == 0 {
		pl1 = watts
	}
	if pl2 == 0 {
		pl2 = watts
	}
	if pl3 == 0 {
		pl3 = watts
	}
	if err := writeIntFile(pptBasePath+"/ppt_pl1_spl", pl1); err != nil {
		return fmt.Errorf("writing ppt_pl1_spl: %w", err)
	}
	if err := writeIntFile(pptBasePath+"/ppt_pl2_sppt", pl2); err != nil {
		return fmt.Errorf("writing ppt_pl2_sppt: %w", err)
	}
	if err := writeIntFile(pptBasePath+"/ppt_fppt", pl3); err != nil {
		return fmt.Errorf("writing ppt_fppt: %w", err)
	}
	if err := writeIntFile(pptBasePath+"/ppt_apu_sppt", pl2); err != nil {
		return fmt.Errorf("writing ppt_apu_sppt: %w", err)
	}
	if err := writeIntFile(pptBasePath+"/ppt_platform_sppt", pl2); err != nil {
		return fmt.Errorf("writing ppt_platform_sppt: %w", err)
	}
	return nil
}
