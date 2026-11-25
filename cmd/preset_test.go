package cmd

import (
	"testing"

	"github.com/dahui/z13ctl/api"
)

func TestEditPreset(t *testing.T) {
	flags := presetEditCmd.Flags()
	for name, value := range map[string]string{"tdp": "60:70", "undervolt": "none", "boost": "off", "refresh": "60"} {
		if err := flags.Set(name, value); err != nil {
			t.Fatal(err)
		}
	}

	p := api.Preset{
		Profile:   "balanced",
		Undervolt: &api.UndervoltState{CPUCO: -10},
		CPUPower:  &api.CPUPowerPreset{MinFrequencyKHz: 600000, EPP: "power", Boost: true},
	}
	if err := editPreset(presetEditCmd, &p); err != nil {
		t.Fatal(err)
	}
	if p.TDP == nil || p.TDP.PL1SPL != 60 || p.TDP.PL2SPPT != 70 || p.TDP.FPPT != 70 || p.TDP.APUSPPT != 70 {
		t.Errorf("TDP = %+v, want 60/70/70 with sPPT following PL2", p.TDP)
	}
	if p.Undervolt != nil || p.CPUPower.Boost || p.CPUPower.EPP != "power" || p.RefreshRate != 60 || p.Profile != "balanced" {
		t.Errorf("edited preset = %+v", p)
	}

	if err := flags.Set("tdp", "1:2:3:4"); err != nil {
		t.Fatal(err)
	}
	if err := editPreset(presetEditCmd, &p); err == nil {
		t.Error("four TDP values accepted")
	}
}
