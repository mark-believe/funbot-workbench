package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// This endpoint only reads reports produced by the independently scheduled simulator.
func (s *Server) handleDailyReport(w http.ResponseWriter, r *http.Request) {
	authorized := false
	for _, seat := range s.ks.Meta() {
		if seat.Kind == "master" && strings.EqualFold(seat.Address, mandateMaster.Hex()) {
			authorized = true
			break
		}
	}
	if !authorized {
		s.writeJSON(w, 200, map[string]any{"available": false})
		return
	}
	exe, err := os.Executable()
	if err != nil {
		s.fail(w, 500, err)
		return
	}
	root := filepath.Join(filepath.Dir(exe), "福泽模拟")
	report := map[string]any{"available": true}
	for _, file := range []string{"latest", "status"} {
		b, err := os.ReadFile(filepath.Join(root, file+".json"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			s.fail(w, 500, err)
			return
		}
		if !json.Valid(b) {
			http.Error(w, "模拟报告格式错误", 500)
			return
		}
		report[file] = json.RawMessage(b)
	}
	s.writeJSON(w, 200, report)
}
