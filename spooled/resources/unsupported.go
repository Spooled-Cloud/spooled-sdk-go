package resources

import (
	"errors"
	"fmt"
)

// ErrUnsupported is matched by errors.Is for every error returned by an SDK
// method whose operation the Spooled API does not offer.
//
// Those methods are kept so existing code still compiles, but they fail at once
// and without a network call, with a message that says why. Before they were
// kept this way each one sent a request the API answered with a bare 404 (or a
// permanent 422) that looked like a transient server fault.
var ErrUnsupported = errors.New("not supported by the Spooled API")

// UnsupportedError is returned by a deprecated SDK method that has no API route
// behind it. It matches ErrUnsupported with errors.Is.
type UnsupportedError struct {
	// Operation names the SDK call, e.g. "Jobs.UpdateProgress".
	Operation string
	// Reason says what to do instead.
	Reason string
}

// Error implements the error interface.
func (e *UnsupportedError) Error() string {
	if e.Reason == "" {
		return fmt.Sprintf("spooled: %s is %s", e.Operation, ErrUnsupported)
	}
	return fmt.Sprintf("spooled: %s is %s: %s", e.Operation, ErrUnsupported, e.Reason)
}

// Is reports a match for ErrUnsupported.
func (e *UnsupportedError) Is(target error) bool { return target == ErrUnsupported }

// IsUnsupportedError returns true if err is (or wraps) an UnsupportedError.
func IsUnsupportedError(err error) bool { return errors.Is(err, ErrUnsupported) }
