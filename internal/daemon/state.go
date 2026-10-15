package daemon

// state.go — State persistence via $XDG_STATE_HOME.
//
// State and LightingState types are defined in the public api package
// (github.com/dahui/z13ctl/api) and used here for persistence.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/dahui/z13ctl/api"
)

var stateSaveMu sync.Mutex

func defaultTabletSettings() api.TabletSettings {
	return api.TabletSettings{
		DisableTouchscreenInDesktop: true,
		TwoFingerHoldContextMenu:    false,
		TouchpadEnabled:             false,
		ScrollSensitivity:           3,
		ScrollSpeed:                 2,
	}
}

func defaultState() api.State {
	return api.State{
		SchemaVersion:  api.StateSchemaVersion,
		Profile:        "balanced",
		TabletSettings: defaultTabletSettings(),
		Lighting: api.LightingState{
			Enabled:    true,
			Mode:       "static",
			Color:      "FF0000",
			Color2:     "000000",
			Speed:      "normal",
			Brightness: 3,
		},
	}
}

// StatePath returns the XDG-compliant path for the z13ctl-plus state file.
func StatePath() string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "z13ctl-plus", "state.json")
}

// LegacyStatePath returns the state path used before the v2 namespace change.
func LegacyStatePath() string {
	return filepath.Join(filepath.Dir(filepath.Dir(StatePath())), "z13ctl", "state.json")
}

// MigrateLegacyState validates and copies legacy state without overwriting plus state.
func MigrateLegacyState() error {
	source := LegacyStatePath()
	destination := StatePath()

	data, err := os.ReadFile(source)
	if os.IsNotExist(err) {
		return fmt.Errorf("legacy state does not exist: %s", source)
	}
	if err != nil {
		return fmt.Errorf("read legacy state: %w", err)
	}
	var state *api.State
	if unmarshalErr := json.Unmarshal(data, &state); unmarshalErr != nil || state == nil {
		return fmt.Errorf("legacy state is not valid JSON")
	}
	if state.SchemaVersion > api.StateSchemaVersion {
		return fmt.Errorf("legacy state schema %d is newer than supported schema %d", state.SchemaVersion, api.StateSchemaVersion)
	}

	if mkdirErr := os.MkdirAll(filepath.Dir(destination), 0o750); mkdirErr != nil {
		return fmt.Errorf("create state directory: %w", mkdirErr)
	}
	tmp, err := os.CreateTemp(filepath.Dir(destination), ".state.json.migrate-*")
	if err != nil {
		return fmt.Errorf("create temporary state file: %w", err)
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write temporary state file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary state file: %w", err)
	}
	if err := os.Link(tmp.Name(), destination); os.IsExist(err) {
		return fmt.Errorf("z13ctl-plus state already exists: %s", destination)
	} else if err != nil {
		return fmt.Errorf("install migrated state: %w", err)
	}
	return nil
}

// loadState reads persisted state. The bool reports whether no state file
// exists yet; malformed existing state must not be treated as a fresh install.
func loadState() (api.State, bool, error) {
	data, err := os.ReadFile(StatePath())
	if err != nil {
		if os.IsNotExist(err) {
			return defaultState(), true, nil
		}
		return api.State{}, false, err
	}
	var s api.State
	if err := json.Unmarshal(data, &s); err != nil {
		return api.State{}, false, fmt.Errorf("parse state: %w", err)
	}
	if s.SchemaVersion > api.StateSchemaVersion {
		return api.State{}, false, fmt.Errorf("state schema %d is newer than supported schema %d", s.SchemaVersion, api.StateSchemaVersion)
	}
	s.TabletIntegration = nil
	changed := migrateState(&s, readProfileFromSysfs())
	if !isPhysicalProfile(s.Profile) {
		s.Profile = "balanced"
		changed = true
	}
	if changed {
		_ = saveState(s)
	}
	return s, false, nil
}

func migrateState(s *api.State, physicalProfile string) bool {
	changed := false
	if s.SchemaVersion < 1 {
		legacyCustom := s.Profile == "custom"
		if legacyCustom {
			if !isPhysicalProfile(physicalProfile) {
				physicalProfile = "balanced"
			}
			s.Profile = physicalProfile
			s.FanCurveActive = s.FanCurve != nil
			s.TDPActive = s.TDP != nil
			s.UndervoltActive = s.Undervolt != nil
		} else {
			if !isPhysicalProfile(s.Profile) {
				s.Profile = "balanced"
			}
			s.FanCurve = nil
			s.TDP = nil
			s.Undervolt = nil
		}
		if s.Undervolt != nil {
			s.Undervolt.Active = s.UndervoltActive
		}

		for name, preset := range s.Presets {
			switch {
			case preset.Profile == "custom":
				preset.Profile = preset.BaseProfile
				if !isPhysicalProfile(preset.Profile) {
					preset.Profile = "balanced"
				}
			case isPhysicalProfile(preset.Profile):
				preset.FanCurve = nil
				preset.TDP = nil
				preset.Undervolt = nil
			default:
				preset.Profile = "balanced"
				preset.FanCurve = nil
				preset.TDP = nil
				preset.Undervolt = nil
			}
			preset.BaseProfile = ""
			s.Presets[name] = preset
		}

		s.SchemaVersion = 1
		changed = true
	}
	if s.SchemaVersion < 2 {
		s.TabletSettings = defaultTabletSettings()
		s.SchemaVersion = 2
		changed = true
	}
	return changed
}

func isPhysicalProfile(profile string) bool {
	return profile == "quiet" || profile == "balanced" || profile == "performance"
}

// saveState atomically writes state to disk.
func saveState(s api.State) error {
	stateSaveMu.Lock()
	defer stateSaveMu.Unlock()
	path := StatePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	s.TabletIntegration = nil
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	// fsync before rename: otherwise a power loss can leave an empty state.json,
	// which loadState rejects and the daemon would never start again.
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// persistState saves the live state while holding d.mu, so a concurrent
// mutation cannot slip in between the marshal and the disk write (lost
// update). Callers must not hold d.mu.
func (d *Daemon) persistState() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return saveState(d.state)
}

func projectTabletSettings(settings api.TabletSettings) error {
	path := api.TabletSettingsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
