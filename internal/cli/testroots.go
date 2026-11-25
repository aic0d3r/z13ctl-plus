package cli

// testroots.go — the sysfs roots the readers and writers use. They are
// variables only so tests in other packages can point them at a temp tree.

var (
	pptBasePath        = "/sys/devices/platform/asus-nb-wmi"
	hwmonDir           = "/sys/class/hwmon"
	platformProfileDir = "/sys/class/platform-profile"
	acpiProfilePath    = "/sys/firmware/acpi/platform_profile"
	cpuFreqDir         = "/sys/devices/system/cpu/cpufreq"
	armouryAttrDir     = "/sys/class/firmware-attributes/asus-armoury/attributes"
	powerSupplyDir     = "/sys/class/power_supply"
	usbDevicesDir      = "/sys/bus/usb/devices"
	smuDriverPath      = "/sys/kernel/ryzen_smu_drv"
)

// UseSysfsRoots redirects every sysfs root to a subpath of root: "ppt",
// "hwmon", "platform-profile", "acpi_platform_profile", "cpufreq", "armoury",
// "power_supply", "usb", "ryzen_smu_drv". Directories are not created. The
// returned func restores the real paths. Tests only; not safe for concurrent use.
func UseSysfsRoots(root string) (restore func()) {
	ptrs := []*string{&pptBasePath, &hwmonDir, &platformProfileDir, &acpiProfilePath,
		&cpuFreqDir, &armouryAttrDir, &powerSupplyDir, &usbDevicesDir, &smuDriverPath}
	names := []string{"ppt", "hwmon", "platform-profile", "acpi_platform_profile",
		"cpufreq", "armoury", "power_supply", "usb", "ryzen_smu_drv"}
	old := make([]string, len(ptrs))
	for i, p := range ptrs {
		old[i], *p = *p, root+"/"+names[i]
	}
	return func() {
		for i, p := range ptrs {
			*p = old[i]
		}
	}
}
