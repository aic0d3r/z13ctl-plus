package cli

// testroots.go — lets tests in other packages point the PPT and hwmon sysfs
// readers at a temporary directory instead of real hardware.

// UseSysfsRoots redirects the asus-nb-wmi PPT directory and the hwmon class
// directory. The returned func restores the real paths. Tests only; not safe
// for concurrent use.
func UseSysfsRoots(ppt, hwmon string) (restore func()) {
	oldPPT, oldHwmon := pptBasePath, hwmonDir
	pptBasePath, hwmonDir = ppt, hwmon
	return func() { pptBasePath, hwmonDir = oldPPT, oldHwmon }
}
