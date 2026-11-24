package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const vipEndpoint = "https://markai.club/api/v1/apps/funbot-workbench/vip"

var vipCodePattern = regexp.MustCompile("^[A-Za-z0-9_-]{32}$")

type vipRequest struct {
	Device string `json:"device"`
	Code   string `json:"code,omitempty"`
}

type vipStatus struct {
	Active *bool `json:"active"`
}

func vipDevice() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(root, "MARKAI", "Workbench", "vip", "device.key")
	release, err := lockVault(path)
	if err != nil {
		return "", err
	}
	defer release()
	secret, err := readPrivateFile(path)
	if os.IsNotExist(err) {
		secret = make([]byte, 32)
		if _, err = rand.Read(secret); err == nil {
			err = atomicPrivateWrite(path, secret)
		}
	}
	if err != nil {
		return "", err
	}
	if len(secret) != 32 {
		return "", fmt.Errorf("本机 VIP 设备凭证损坏")
	}
	machine, err := vipMachineID()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(append(secret, []byte(machine)...))
	return hex.EncodeToString(sum[:]), nil
}

func requestVIP(ctx context.Context, endpoint string, input vipRequest) (vipStatus, error) {
	var result vipStatus
	raw, err := json.Marshal(input)
	if err != nil {
		return result, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return result, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := http.Client{Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		return result, fmt.Errorf("无法连接 MARK·AI，请稍后重试")
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusConflict, http.StatusUnprocessableEntity:
		return result, fmt.Errorf("激活码已使用、过期或不可用")
	case http.StatusOK:
		if err = json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&result); err == nil && result.Active != nil {
			return result, nil
		}
	}
	return result, fmt.Errorf("MARK·AI VIP 服务暂不可用")
}

func (s *Server) handleVIP(activate bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input vipRequest
		if activate {
			var body struct {
				Code string `json:"code"`
			}
			if err := s.body(r, &body); err != nil {
				s.fail(w, 400, err)
				return
			}
			input.Code = strings.TrimSpace(body.Code)
			if !vipCodePattern.MatchString(input.Code) {
				s.fail(w, 400, fmt.Errorf("请输入完整的 32 位激活码"))
				return
			}
		}
		var err error
		input.Device, err = vipDevice()
		if err != nil {
			s.fail(w, 500, fmt.Errorf("无法读取本机 VIP 设备凭证"))
			return
		}
		status, err := requestVIP(r.Context(), vipEndpoint, input)
		if err != nil {
			s.fail(w, 400, err)
			return
		}
		s.writeJSON(w, 200, status)
	}
}
