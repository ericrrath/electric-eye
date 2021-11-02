// SPDX-FileCopyrightText: 2021 Eric R. Rath
// SPDX-License-Identifier: MPL-2.0

package util

import (
	"syscall"

	"k8s.io/klog/v2"
)

func EnsureFileDescriptorLimit(limit uint64) error {
	var rLimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
		return err
	}
	if rLimit.Cur >= limit {
		klog.V(4).Infof("file descriptor limits already sufficient; want %d, have: %v", limit, rLimit)
		return nil
	}
	klog.Infof("file descriptor limits insufficient: want %d, have: %v", limit, rLimit)
	// each poll will require two network accesses: 1) DSN lookup, and 2) HTTP request
	rLimit.Cur = limit
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
		return err
	}
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
		return err
	}
	klog.Infof("updated file descriptor limits: %v", rLimit)
	return nil
}
