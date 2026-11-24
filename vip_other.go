//go:build !windows

package main

import "os"

func vipMachineID() (string, error) {
	return os.Hostname()
}
