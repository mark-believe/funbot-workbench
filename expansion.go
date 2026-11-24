package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func (s *Server) expansion(open bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authorized := false
		for _, seat := range s.ks.Meta() {
			if seat.Kind == "master" && strings.EqualFold(seat.Address, mandateMaster.Hex()) {
				authorized = true
				break
			}
		}
		if !authorized {
			s.fail(w, 403, fmt.Errorf("此独立账本绑定指定账号组 的主号"))
			return
		}
		exe, err := os.Executable()
		if err != nil {
			s.fail(w, 500, err)
			return
		}
		dir := filepath.Join(filepath.Dir(exe), "扩组核算")
		if open {
			if runtime.GOOS != "windows" {
				s.fail(w, 400, fmt.Errorf("请在本机打开分组核算.xlsx"))
				return
			}
			book := filepath.Join(dir, "分组核算.xlsx")
			if _, err = os.Stat(book); err != nil {
				s.fail(w, 500, err)
				return
			}
			cmd := exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", book)
			if err = cmd.Start(); err != nil {
				s.fail(w, 500, err)
				return
			}
			go cmd.Wait()
			s.ok(w)
			return
		}
		manifest, err := os.ReadFile(filepath.Join(dir, "groups.json"))
		if err != nil || !json.Valid(manifest) {
			s.fail(w, 500, fmt.Errorf("无法读取建表名册"))
			return
		}
		home, err := os.UserHomeDir()
		if err != nil {
			s.fail(w, 500, err)
			return
		}
		python := filepath.Join(home, ".cache", "codex-runtimes", "codex-primary-runtime", "dependencies", "python", "pythonw.exe")
		if _, err := os.Stat(python); os.IsNotExist(err) {
			python, err = exec.LookPath("python")
			if err != nil {
				s.fail(w, 503, fmt.Errorf("读取工作簿需要安装 Python 3 和 openpyxl"))
				return
			}
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, python, "-B", filepath.Join(dir, "evaluate.py"), "--read").Output()
		if err != nil || !json.Valid(out) {
			s.fail(w, 500, fmt.Errorf("无法读取独立核算表，请检查本地工作簿与 Python 运行时"))
			return
		}
		s.writeJSON(w, 200, map[string]any{"book": json.RawMessage(out), "manifest": json.RawMessage(manifest)})
	}
}
