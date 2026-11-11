package cli

import (
	"encoding/binary"
	"testing"
	"time"
)

// Inside the quiet window the NPU must not be queried: with no device present
// a real query would fail, so getting the saved reading proves it was skipped.
func TestReadNPUTelemetryReusesQuietReading(t *testing.T) {
	npuQuietMu.Lock()
	npuQuietAt, npuQuietPower, npuQuietClock = time.Now(), 0.42, 1267
	npuQuietMu.Unlock()
	t.Cleanup(func() {
		npuQuietMu.Lock()
		npuQuietAt = time.Time{}
		npuQuietMu.Unlock()
	})

	power, util, clock, err := ReadNPUTelemetry()
	if err != nil || power != 0.42 || util != 0 || clock != 1267 {
		t.Fatalf("ReadNPUTelemetry() = %v, %d, %d, %v; want the saved quiet reading", power, util, clock, err)
	}
}

func TestReadMemory(t *testing.T) {
	used, total, err := ReadMemory()
	if err != nil {
		t.Fatal(err)
	}
	if total <= 0 || used < 0 || used > total {
		t.Fatalf("invalid memory usage: %d / %d MiB", used, total)
	}
}

func TestParseGPUCurrentUClockMHz(t *testing.T) {
	data := make([]byte, 264)
	binary.LittleEndian.PutUint16(data[:2], uint16(len(data)))
	data[2], data[3] = 3, 0
	binary.LittleEndian.PutUint16(data[186:188], 425)

	got, err := parseGPUCurrentUClockMHz(data)
	if err != nil {
		t.Fatal(err)
	}
	if got != 425 {
		t.Fatalf("parseGPUCurrentUClockMHz() = %d, want 425", got)
	}
}

func TestParseGPUCurrentUClockMHzRejectsUnsupportedRevision(t *testing.T) {
	data := make([]byte, 264)
	binary.LittleEndian.PutUint16(data[:2], uint16(len(data)))
	data[2], data[3] = 2, 4

	if _, err := parseGPUCurrentUClockMHz(data); err == nil {
		t.Fatal("parseGPUCurrentUClockMHz() accepted unsupported revision")
	}
}

func TestParseGPUPowerW(t *testing.T) {
	data := make([]byte, 264)
	binary.LittleEndian.PutUint16(data[:2], uint16(len(data)))
	data[2], data[3] = 3, 0
	binary.LittleEndian.PutUint32(data[124:128], 5123)

	got, err := parseGPUPowerW(data)
	if err != nil {
		t.Fatal(err)
	}
	if got != 5.123 {
		t.Fatalf("parseGPUPowerW() = %v, want 5.123", got)
	}
}
