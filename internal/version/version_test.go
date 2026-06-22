package version

import (
	"testing"
)

func TestVersionIsSet(t *testing.T) {
	if Version == "" {
		t.Fatal("Version must not be empty")
	}
}

func TestVersionFormat(t *testing.T) {
	if Version[0] == 'v' {
		t.Logf("version is tagged format: %s", Version)
	} else {
		t.Logf("version is dev format: %s", Version)
	}
}
