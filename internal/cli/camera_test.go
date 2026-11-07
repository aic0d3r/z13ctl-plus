package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindCameraIn(t *testing.T) {
	root := t.TempDir()
	device := func(name, removable, product string) {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for file, value := range map[string]string{"removable": removable, "product": product, "authorized": "1"} {
			if err := os.WriteFile(filepath.Join(dir, file), []byte(value+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	device("1-2", "removable", "USB Webcam")     // external webcam: left alone
	device("3-3", "fixed", "Wireless_Device")    // internal, not a camera
	device("1-1:1.0", "fixed", "ASUS 5M webcam") // interface entry
	if got := findCameraIn(root); got != "" {
		t.Fatalf("found %q, want none", got)
	}
	device("1-1", "fixed", "ASUS 5M webcam")
	if got, want := findCameraIn(root), filepath.Join(root, "1-1", "authorized"); got != want {
		t.Fatalf("found %q, want %q", got, want)
	}
}
