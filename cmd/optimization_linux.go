//go:build linux
// +build linux

package cmd

import (
	"syscall"
)

func applyLinuxRlimit(logger interface {
	Debugf(string, ...interface{})
	Errorf(string, ...interface{})
}) {
	// Set file descriptor limit programmatically
	var rLimit syscall.Rlimit
	err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit)
	if err != nil {
		logger.Errorf("Error getting Rlimit: %v", err)
	} else {
		logger.Debugf("Current file descriptor limit: %d", rLimit.Cur)

		// Set the maximum and current file descriptor limits to 1048576
		rLimit.Max = 1048576
		rLimit.Cur = 1048576
		err = syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit)
		if err != nil {
			logger.Errorf("Error setting Rlimit: %v", err)
		} else {
			logger.Debugf("Successfully set file descriptor limit to: %d", rLimit.Cur)
		}
	}
}
