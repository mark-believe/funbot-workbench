package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Both workbenches must be idle. Equal files make retry after interruption safe.
// Publish complete files without replacing any existing destination.
func migrateWorkbench(source, targetKey string) error {
	oldKey := filepath.Join(source, ".data", "keys.json")
	if err := regularFile(oldKey); err != nil {
		return err
	}
	if _, err := os.Stat(oldKey); err != nil {
		return fmt.Errorf("无法读取旧工作台的 .data/keys.json: %w", err)
	}
	if err := initializeVaultLocation(targetKey); err != nil {
		return err
	}
	for _, path := range []string{oldKey, targetKey} {
		release, err := lockSystem(path)
		if err != nil {
			return fmt.Errorf("请先关闭新旧工作台，并选择不同目录: %w", err)
		}
		defer release()
		unlock, err := lockVault(path)
		if err != nil {
			return err
		}
		defer unlock()
	}
	type file struct{ src, dst string }
	var files []file
	for _, suffix := range []string{".settings.json", ".pending.json"} {
		if _, err := os.Stat(oldKey + suffix); err == nil {
			files = append(files, file{oldKey + suffix, targetKey + suffix})
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	root := filepath.Dir(filepath.Dir(targetKey))
	for _, folder := range []string{"福泽模拟", "扩组核算"} {
		dir := filepath.Join(source, folder)
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		for _, e := range entries {
			name := e.Name()
			if !e.IsDir() && (strings.HasSuffix(name, ".json") || name == "分组核算.xlsx") {
				files = append(files, file{filepath.Join(dir, name), filepath.Join(root, folder, name)})
			}
		}
	}
	files = append(files, file{oldKey, targetKey})
	pending := map[string][]byte{}
	for _, f := range files {
		if err := regularFile(f.src); err != nil {
			return err
		}
		data, err := os.ReadFile(f.src)
		if err != nil {
			return err
		}
		if f.src == oldKey {
			var v Vault
			if err = json.Unmarshal(data, &v); err != nil {
				return fmt.Errorf("旧密钥文件格式无效")
			}
			if err = validateVault(&v); err != nil {
				return err
			}
		}
		if err = regularFile(f.dst); err != nil {
			return err
		}
		existing, err := os.ReadFile(f.dst)
		if err == nil {
			if !bytes.Equal(existing, data) {
				return fmt.Errorf("目标已含不同数据，请解压到新目录后迁移：%s", f.dst)
			}
			continue
		}
		if !os.IsNotExist(err) {
			return err
		}
		pending[f.dst] = data
	}
	for _, f := range files {
		data, ok := pending[f.dst]
		if !ok {
			continue
		}
		if err := publishMigratedFile(f.dst, data); err != nil {
			return err
		}
	}
	return nil
}
func publishMigratedFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".migration-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	defer tmp.Close()
	if err = protectFile(name); err != nil {
		return err
	}
	if _, err = tmp.Write(data); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Link(name, path); err != nil {
		return err
	}
	return syncDirectory(dir)
}
