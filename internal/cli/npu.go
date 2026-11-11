package cli

// npu.go — AMD Ryzen AI NPU (XDNA) live telemetry via the amdxdna DRM accel
// driver (mainline since Linux 6.19; amdxdna-dkms 7.0+ on older kernels).
//
// Reads power (W) and average per-column utilization (%) via the
// DRM_AMDXDNA_QUERY_SENSORS ioctl, plus the MP-NPU clock via
// DRM_AMDXDNA_QUERY_CLOCK_METADATA. Both are hardware-backed sensors
// exported by the driver — no derived submission-counter deltas, no
// xrt-smi / xdna-top dependency.
//
// Device discovery: /sys/class/accel/accel*/device with vendor 0x1022 (AMD).
// The accel char device (/dev/accel/accelN) is opened per call to match the
// amdgpu sysfs reader pattern; the cost is one extra open per telemetry pull.
// Requires render-group access (same perms as the amdgpu DRM node).
//
// We deliberately do NOT expose the amdxdna DPM power_mode interface
// (DEFAULT/HIGH/TURBO via DRM_AMDXDNA_SET_STATE). Empirical A/B testing
// (Qwen3-0.6B sustained + bursty workloads) showed all three modes draw
// within 10-20mW of each other — the driver pins DPM at max whenever any
// process holds the device open, so the mode setting has no measurable
// effect. Only "is the device open?" moves NPU power, and that's an
// application behavior, not a mode we can toggle.

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

const (
	// _IOWR('d', 0x40+7, struct amdxdna_drm_get_info) on x86_64 Linux.
	// dir=3 (RW), magic='d'=0x64, nr=0x47, size=16 -> 0xC0106447.
	amdxdnaIoctlGetInfo = 0xC0106447

	drmAmdxdnaQueryClockMetadata = 3 // enum amdxdna_drm_get_param
	drmAmdxdnaQuerySensors       = 4

	amdxdnaSensorPower             = 0 // enum amdxdna_sensor_type
	amdxdnaSensorColumnUtilization = 1

	amdxdnaSensorSize  = 168 // unsafe.Sizeof(amdxdnaQuerySensor{})
	amdxdnaClockMDSize = 48  // unsafe.Sizeof(amdxdnaQueryClockMetadata{})
)

// amdxdnaDrmGetInfo mirrors struct amdxdna_drm_get_info (16 bytes).
// buffer is unsafe.Pointer (not uint64) so the GC keeps the backing slice
// alive across the syscall; the byte layout matches the kernel's __u64.
type amdxdnaDrmGetInfo struct {
	param      uint32
	bufferSize uint32
	buffer     unsafe.Pointer
}

// amdxdnaQuerySensor mirrors struct amdxdna_drm_query_sensor (168 bytes).
type amdxdnaQuerySensor struct {
	label   [64]byte
	input   uint32
	max     uint32
	average uint32
	highest uint32
	status  [64]byte
	units   [16]byte
	unitm   int8
	typ     uint8
	pad     [6]byte
}

// amdxdnaQueryClock mirrors struct amdxdna_drm_query_clock (24 bytes).
type amdxdnaQueryClock struct {
	name    [16]byte
	freqMHz uint32
	pad     uint32
}

// amdxdnaQueryClockMetadata mirrors struct amdxdna_drm_query_clock_metadata
// (48 bytes): two amdxdna_drm_query_clock structs.
type amdxdnaQueryClockMetadata struct {
	mpNpuClock amdxdnaQueryClock
	hClock     amdxdnaQueryClock
}

// npuDeviceOnce + npuDevicePath cache the discovered accel device path.
var (
	npuDeviceOnce         sync.Once
	npuDevicePath         string
	npuSensorsUnsupported atomic.Bool
)

// findNPUDevice returns the /dev/accel/accelN path for the AMD XDNA NPU,
// or "" if no AMD accel device is bound. Cached on first call.
func findNPUDevice() string {
	npuDeviceOnce.Do(func() {
		const sysClass = "/sys/class/accel"
		entries, err := os.ReadDir(sysClass)
		if err != nil {
			return
		}
		for _, e := range entries {
			name := e.Name()
			if !strings.HasPrefix(name, "accel") {
				continue
			}
			data, err := os.ReadFile(sysClass + "/" + name + "/device/vendor")
			if err != nil {
				continue
			}
			if strings.TrimSpace(string(data)) != "0x1022" {
				continue
			}
			npuDevicePath = "/dev/accel/" + name
			return
		}
	})
	return npuDevicePath
}

// npuQuery issues DRM_AMDXDNA_GET_INFO with the given param. The driver
// writes the actual response size back into req.bufferSize; we return that
// trimmed slice. Starts with a generous buffer (16 sensors worth) and retries
// once if the driver signals it needs more space.
func npuQuery(param uint32) ([]byte, error) {
	path := findNPUDevice()
	if path == "" {
		return nil, fmt.Errorf("amdxdna NPU not found")
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	size := 16 * amdxdnaSensorSize
	for range 2 {
		buf := make([]byte, size)
		req := amdxdnaDrmGetInfo{
			param:      param,
			bufferSize: uint32(size),
			buffer:     unsafe.Pointer(&buf[0]),
		}
		_, _, errno := syscall.Syscall(
			syscall.SYS_IOCTL, f.Fd(),
			uintptr(amdxdnaIoctlGetInfo),
			uintptr(unsafe.Pointer(&req)),
		)
		if errno != 0 {
			// Driver returns ERANGE with the needed size written back when
			// the caller-supplied buffer is too small.
			if errno == syscall.ERANGE && int(req.bufferSize) > size {
				size = int(req.bufferSize)
				continue
			}
			return nil, fmt.Errorf("amdxdna ioctl %d: %w", param, errno)
		}
		return buf[:int(req.bufferSize)], nil
	}
	return nil, fmt.Errorf("amdxdna ioctl %d: buffer retry exhausted", param)
}

func npuRuntimeActive() (bool, error) {
	device := findNPUDevice()
	if device == "" {
		return false, fmt.Errorf("amdxdna NPU not found")
	}
	name := strings.TrimPrefix(device, "/dev/accel/")
	status, err := os.ReadFile("/sys/class/accel/" + name + "/device/power/runtime_status")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(status)) == "active", nil
}

// ReadNPUSensors returns NPU power draw and average per-column utilization.
// An idle NPU is unavailable. On drivers without the sensor query, an active
// NPU is reported with zero-valued power and utilization.
func ReadNPUSensors() (powerW float64, utilPct int, err error) {
	if active, runtimeErr := npuRuntimeActive(); runtimeErr == nil && !active {
		return 0, 0, fmt.Errorf("amdxdna NPU is idle")
	}
	if npuSensorsUnsupported.Load() {
		return 0, 0, nil
	}
	payload, err := npuQuery(drmAmdxdnaQuerySensors)
	if errors.Is(err, syscall.EOPNOTSUPP) {
		npuSensorsUnsupported.Store(true)
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	var utilSum, utilCount int
	for off := 0; off+amdxdnaSensorSize <= len(payload); off += amdxdnaSensorSize {
		s := (*amdxdnaQuerySensor)(unsafe.Pointer(&payload[off]))
		switch s.typ {
		case amdxdnaSensorPower:
			v := float64(s.input)
			units := strings.ToLower(strings.TrimSpace(string(s.units[:])))
			if s.unitm < 0 || strings.HasPrefix(units, "m") {
				v /= 1000.0
			}
			powerW = v
		case amdxdnaSensorColumnUtilization:
			utilSum += int(s.input)
			utilCount++
		}
	}
	if utilCount > 0 {
		utilPct = utilSum / utilCount
	}
	return powerW, utilPct, nil
}

// npuQuietWindow must exceed the driver's 5 s runtime-PM autosuspend delay.
// Every ioctl restarts that timer, so a client polling once a second would
// otherwise keep an unused NPU powered for good.
const npuQuietWindow = 7 * time.Second

var (
	npuQuietMu    sync.Mutex
	npuQuietAt    time.Time // when the last 0% reading was taken; zero = none
	npuQuietPower float64
	npuQuietClock int
)

// ReadNPUTelemetry returns power, utilization and clock for the NPU. After a
// reading with 0% utilization the device is not queried again for
// npuQuietWindow (the saved reading is returned), so it can autosuspend. An
// NPU that is still busy afterwards belongs to someone else's workload.
func ReadNPUTelemetry() (powerW float64, utilPct, clockMHz int, err error) {
	npuQuietMu.Lock()
	defer npuQuietMu.Unlock()
	if !npuQuietAt.IsZero() && time.Since(npuQuietAt) < npuQuietWindow {
		return npuQuietPower, 0, npuQuietClock, nil
	}
	npuQuietAt = time.Time{}
	powerW, utilPct, err = ReadNPUSensors()
	if err != nil {
		return 0, 0, 0, err
	}
	if !npuSensorsUnsupported.Load() {
		if v, clockErr := ReadNPUClockMHz(); clockErr == nil {
			clockMHz = v
		}
	}
	if utilPct == 0 {
		npuQuietAt, npuQuietPower, npuQuietClock = time.Now(), powerW, clockMHz
	}
	return powerW, utilPct, clockMHz, nil
}

// ReadNPUClockMHz returns the current MP-NPU clock (npuclk) in MHz, or
// (0, err) if the NPU or clock query is unavailable. This is the live npuclk
// from the current DPM level (driver updates ndev->npuclk_freq on every DPM
// switch — see aie2_smu.c). In practice on Strix Halo the driver pins DPM
// at max (1267 MHz) whenever any process holds /dev/accel/accelN open, so
// the value may appear static — but it is not metadata.
func ReadNPUClockMHz() (int, error) {
	payload, err := npuQuery(drmAmdxdnaQueryClockMetadata)
	if err != nil {
		return 0, err
	}
	if len(payload) < amdxdnaClockMDSize {
		return 0, fmt.Errorf("amdxdna clock metadata: short response (%d)", len(payload))
	}
	md := (*amdxdnaQueryClockMetadata)(unsafe.Pointer(&payload[0]))
	return int(md.mpNpuClock.freqMHz), nil
}
