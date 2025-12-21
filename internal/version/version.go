// Package version provides SDK version information.
package version

import (
	"fmt"
	"runtime"
)

// SDK version constants
const (
	// Version is the current SDK version.
	Version = "1.0.9"

	// SDKName is the name of the SDK.
	SDKName = "spooled-go"
)

// UserAgent returns the default user agent string for the SDK.
func UserAgent() string {
	return fmt.Sprintf("%s/%s (%s; %s)", SDKName, Version, runtime.GOOS, runtime.GOARCH)
}

// ShortUserAgent returns a shorter user agent string.
func ShortUserAgent() string {
	return fmt.Sprintf("%s/%s", SDKName, Version)
}
