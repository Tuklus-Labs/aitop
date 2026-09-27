//go:build !darwin

package proc

import "errors"

var errNativeUnavailable = errors.New("native process collector unavailable")
