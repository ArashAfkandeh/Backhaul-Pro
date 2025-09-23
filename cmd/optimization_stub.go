//go:build !linux
// +build !linux

package cmd

func applyLinuxRlimit(logger interface {
	Debugf(string, ...interface{})
	Errorf(string, ...interface{})
}) {
	// No-op on non-Linux systems
}
