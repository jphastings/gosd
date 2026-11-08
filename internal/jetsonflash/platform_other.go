//go:build !linux

package jetsonflash

import (
	"errors"
	"time"
)

func openRecovery(time.Duration) (conn, error) {
	return nil, errors.New("talking to a module in USB recovery mode needs Linux's usbfs; run jetson-flash on the Turing Pi 2's BMC, or on a Linux PC cabled to the module's recovery port")
}
