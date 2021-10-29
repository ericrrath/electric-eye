package util

import (
	"syscall"

	"k8s.io/klog/v2"
)

func SetFileDescriptorLimit(limit uint64) error {
	var rLimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
		return err
	}
	klog.Infof("file descriptor limits before: %v", rLimit)
	// each poll will require two network accesses: 1) DSN lookup, and 2) HTTP request
	rLimit.Cur = limit
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
		return err
	}
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
		return err
	}
	klog.Infof("file descriptor limits after: %v", rLimit)
	return nil
}
