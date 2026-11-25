package hid

// testdevice.go — a Device backed by any file, so tests in other packages
// (e.g. the daemon) can drive HID handlers without hardware.

import "os"

// NewTestDeviceNamed creates a Device with one node named name at path,
// writing to f. Tests only.
func NewTestDeviceNamed(f *os.File, path, name string) *Device {
	return &Device{nodes: []hidrawNode{{path: path, name: name, f: f}}}
}
