//go:build !windows

package main

func reopenDashboard(string) bool { return false }
func listenForDashboardOpen(string, string) {}
