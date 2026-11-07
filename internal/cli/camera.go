package cli

// camera.go — switch the built-in USB camera on or off through the USB
// device's sysfs "authorized" attribute. Writing 0 makes the kernel unbind the
// device (every /dev/video* node of it disappears, including the IR camera);
// writing 1 re-probes it. The kernel authorizes the device again on every
// boot, so the daemon reapplies a saved "off" at startup and after resume.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const usbDevicesDir = "/sys/bus/usb/devices"

// FindCameraPath returns the "authorized" sysfs file of the built-in camera,
// or "" when none is found.
func FindCameraPath() string {
	return findCameraIn(usbDevicesDir)
}

// findCameraIn matches a USB device (not an interface) that is wired to a
// fixed internal port and names itself a camera or webcam. The descriptors
// stay readable while the device is deauthorized, so it is found either way.
func findCameraIn(root string) string {
	dirs, _ := filepath.Glob(root + "/*")
	for _, dir := range dirs {
		if strings.Contains(filepath.Base(dir), ":") {
			continue // interface, not a device
		}
		if removable, _ := ReadTrimmedFile(dir + "/removable"); removable != "fixed" {
			continue
		}
		product, _ := ReadTrimmedFile(dir + "/product")
		product = strings.ToLower(product)
		if !strings.Contains(product, "webcam") && !strings.Contains(product, "camera") {
			continue
		}
		if path := dir + "/authorized"; pathExists(path) {
			return path
		}
	}
	return ""
}

// ReadCamera returns 1 when the built-in camera is on and 0 when it is off.
func ReadCamera() (int, error) {
	path := FindCameraPath()
	if path == "" {
		return 0, fmt.Errorf("no built-in camera found")
	}
	value, err := ReadTrimmedFile(path)
	if err != nil {
		return 0, err
	}
	if value == "0" {
		return 0, nil
	}
	return 1, nil
}

// SetCamera turns the built-in camera on (1) or off (0).
func SetCamera(value int) error {
	path := FindCameraPath()
	if path == "" {
		return fmt.Errorf("no built-in camera found")
	}
	return os.WriteFile(path, []byte(fmt.Sprintf("%d\n", value)), 0o644)
}
