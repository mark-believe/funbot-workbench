//go:build windows

package main

import (
	"crypto/sha256"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

func dashboardEvent(path string) *uint16 {
	key := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(path))))
	name, _ := windows.UTF16PtrFromString(fmt.Sprintf("Local\\FunbotDashboardOpen-%x", key))
	return name
}

// The second launch signals the owner of this vault to open its own URL.
// The access token stays in the original process.
func reopenDashboard(path string) bool {
	for i := 0; i < 30; i++ {
		h, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, dashboardEvent(path))
		if err == nil {
			defer windows.CloseHandle(h)
			return windows.SetEvent(h) == nil
		}
		if err != windows.ERROR_FILE_NOT_FOUND {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func listenForDashboardOpen(path, url string) {
	event, err := windows.CreateEvent(nil, 0, 0, dashboardEvent(path))
	if err != nil {
		log.Printf("无法启用快捷方式重复打开功能：%v", err)
		return
	}
	go func() {
		defer windows.CloseHandle(event)
		for {
			if _, err := windows.WaitForSingleObject(event, windows.INFINITE); err != nil {
				return
			}
			openBrowser(url)
		}
	}()
}
