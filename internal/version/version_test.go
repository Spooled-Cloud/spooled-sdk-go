package version

import (
	"runtime"
	"strings"
	"testing"
)

func TestVersion(t *testing.T) {
	if Version == "" {
		t.Error("Version should not be empty")
	}
	// Version should be semver-like
	parts := strings.Split(Version, ".")
	if len(parts) < 2 {
		t.Errorf("Version %q should have at least major.minor", Version)
	}
}

func TestSDKName(t *testing.T) {
	if SDKName != "spooled-go" {
		t.Errorf("SDKName should be 'spooled-go', got %q", SDKName)
	}
}

func TestUserAgent(t *testing.T) {
	ua := UserAgent()
	if !strings.Contains(ua, SDKName) {
		t.Errorf("UserAgent should contain SDK name, got %q", ua)
	}
	if !strings.Contains(ua, Version) {
		t.Errorf("UserAgent should contain version, got %q", ua)
	}
	if !strings.Contains(ua, runtime.GOOS) {
		t.Errorf("UserAgent should contain GOOS, got %q", ua)
	}
	if !strings.Contains(ua, runtime.GOARCH) {
		t.Errorf("UserAgent should contain GOARCH, got %q", ua)
	}
}

func TestShortUserAgent(t *testing.T) {
	ua := ShortUserAgent()
	expected := SDKName + "/" + Version
	if ua != expected {
		t.Errorf("ShortUserAgent() = %q, want %q", ua, expected)
	}
}
