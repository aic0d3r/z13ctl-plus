package hid

// device.go — Device type, known device table, and I/O methods.

import (
	"fmt"
	"os"
)

const (
	// ReportSize is the fixed 64-byte output report length used by all Aura commands.
	ReportSize = 64
)

// knownDevices lists all ASUS HID devices that carry Aura (report 0x5d).
// On the 2025 ROG Flow Z13:
//
//	0x18c6 = N-KEY Device     → lightbar
//	0x1a30 = GZ302EA-Keyboard → keyboard
//
// g-helper's AsusHid.Write() broadcasts to ALL matching devices; we do the same.
var knownDevices = map[string]string{
	"HID_ID=0003:00000B05:000018C6": "lightbar",
	"HID_ID=0003:00000B05:00001A30": "keyboard",
}

// hidrawNode is one open hidraw file.
type hidrawNode struct {
	path string
	name string // "keyboard", "lightbar", or "" if unknown
	f    *os.File
}

// Device holds all open hidraw nodes that have the Aura report (0x5d).
// Writes are broadcast to every node, matching g-helper's AsusHid.Write() behavior.
type Device struct {
	nodes []hidrawNode
}

// DeviceInfo describes a discovered hidraw node, for display purposes.
type DeviceInfo struct {
	Path    string // e.g. /dev/hidraw0
	Name    string // "keyboard", "lightbar", or "" if unrecognised
	HasAura bool   // true if the HID descriptor contains Report ID 0x5d
	OpenErr string // non-empty if the device could not be opened
}

// Write sends a 64-byte output report to every Aura node (zero-padded).
func (d *Device) Write(data []byte) error {
	buf := make([]byte, ReportSize)
	copy(buf, data)
	var lastErr error
	for _, n := range d.nodes {
		if _, err := n.f.Write(buf); err != nil {
			lastErr = fmt.Errorf("write to %s: %w", n.path, err)
		}
	}
	return lastErr
}

// Descriptions returns human-readable device descriptions: "path (name)".
func (d *Device) Descriptions() []string {
	descs := make([]string, len(d.nodes))
	for i, n := range d.nodes {
		if n.name != "" {
			descs[i] = fmt.Sprintf("%s (%s)", n.path, n.name)
		} else {
			descs[i] = n.path
		}
	}
	return descs
}

// FilteredView returns a Device that writes only to the node matching nameOrPath.
// nameOrPath may be "keyboard", "lightbar", or a /dev/hidrawN path.
// If nameOrPath is empty, d itself is returned.
// If nameOrPath is non-empty but matches no node, an error is returned.
// The returned *Device shares file descriptors with d; do not call Close on it.
func (d *Device) FilteredView(nameOrPath string) (*Device, error) {
	if nameOrPath == "" {
		return d, nil
	}
	var matched []hidrawNode
	for _, n := range d.nodes {
		if n.name == nameOrPath || n.path == nameOrPath {
			matched = append(matched, n)
		}
	}
	if len(matched) == 0 {
		return nil, fmt.Errorf("device %q not found (available: keyboard, lightbar)", nameOrPath)
	}
	return &Device{nodes: matched}, nil
}

// Holds reports whether d has at least one node named name, and whether all of
// them are still live: their /dev path refers to the opened file. A device
// that re-enumerated gets a new node even when it reuses the hidrawN number,
// so a stale handle is caught even if no poll saw it disappear.
func (d *Device) Holds(name string) (held, live bool) {
	live = true
	for _, n := range d.nodes {
		if n.name != name {
			continue
		}
		held = true
		opened, err := n.f.Stat()
		current, perr := os.Stat(n.path)
		if err != nil || perr != nil || !os.SameFile(opened, current) {
			live = false
		}
	}
	return held, held && live
}

// Close releases all open nodes.
func (d *Device) Close() {
	for _, n := range d.nodes {
		_ = n.f.Close() //nolint:errcheck // best-effort cleanup
	}
}
