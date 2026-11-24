package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type assetDayTotals struct {
	Day        string
	Start, End assetBlock
	Groups     map[int]string
	Total      string
}

func assetDaySummary(day string, first, last *assetSheet) (assetDayTotals, error) {
	r := assetDayTotals{Day: day, Start: first.Block, End: last.Block, Groups: map[int]string{}}
	if len(first.Accounts) != len(last.Accounts) {
		return r, fmt.Errorf("两次快照名册不符")
	}
	total := new(big.Int)
	for i, b := range last.Accounts {
		a := first.Accounts[i]
		if a.Address != b.Address || a.Group != b.Group {
			return r, fmt.Errorf("账号归属变化，请重新读取")
		}
		delta := new(big.Int).Sub(weiValue(b.Available), weiValue(a.Available))
		r.Groups[a.Group] = new(big.Int).Add(weiValue(r.Groups[a.Group]), delta).String()
		total.Add(total, delta)
	}
	r.Total = total.String()
	return r, nil
}
func readAssetManifest(path string) ([]VaultMeta, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m struct{ Groups map[string][]string }
	if err = json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	var seats []VaultMeta
	for g := 1; g <= len(m.Groups); g++ {
		rows := m.Groups[strconv.Itoa(g)]
		if len(rows) == 0 || len(rows) > 24 || (g < len(m.Groups) && len(rows) != 24) {
			return nil, fmt.Errorf("分组名册不完整")
		}
		for _, addr := range rows {
			kind := "member"
			if len(seats) == 0 {
				kind = "master"
			}
			seats = append(seats, VaultMeta{Index: len(seats), Kind: kind, Address: addr})
		}
	}
	return assetRoster(seats)
}
func printAssetDay(day string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	seats, err := readAssetManifest(filepath.Join(filepath.Dir(exe), "扩组核算", "groups.json"))
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	c := NewChain([]string{"https://rpc.anubispace.org", "https://rpc1.anubispace.org", "https://rpc2.anubispace.org"}, mainnetChainID)
	head, err := assetHeader(ctx, c, nil)
	if err != nil {
		return err
	}
	d, err := assetDay(ctx, c, day, head)
	if err != nil {
		return err
	}
	a, err := readAssets(ctx, c, d.Start, seats)
	if err != nil {
		return err
	}
	b, err := readAssets(ctx, c, d.End, seats)
	if err != nil {
		return err
	}
	out, err := assetDaySummary(day, a, b)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(out)
}
