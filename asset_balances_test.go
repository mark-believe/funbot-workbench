package main

import (
	"context"
	"github.com/ethereum/go-ethereum/common"
	"os"
	"reflect"
	"testing"
	"time"
)

func TestAssetAccounting(t *testing.T) {
	// Moving claimable jiazi to another member's wallet does not change team wealth.
	a := assetAccount{Address: "a", Group: 1, Wallet: "10", Jiazi: "8", Lineage: "2", Fuze: "3", Arena: "5", Occupied: "90"}
	if a.sum().String() != "28" {
		t.Fatal(a.sum())
	}
	a.Available = a.sum().String()
	b := assetAccount{Address: "b", Group: 2, Wallet: "0", Available: "0"}
	a2, b2 := a, b
	a2.Jiazi = "0"
	b2.Wallet = "8"
	a2.Available = a2.sum().String()
	b2.Available = b2.sum().String()
	r, e := assetDaySummary("day", &assetSheet{Accounts: []assetAccount{a, b}}, &assetSheet{Accounts: []assetAccount{a2, b2}})
	if e != nil || r.Total != "0" || r.Groups[1] != "-8" || r.Groups[2] != "8" {
		t.Fatal(r, e)
	}
}
func TestAssetStages(t *testing.T) {
	a, b := common.HexToAddress("0x1"), common.HexToAddress("0x2")
	data, e := ledgerABI.Pack("playSome", []common.Address{b, a, b})
	if e != nil {
		t.Fatal(e)
	}
	gs, u := txGroups(data, map[common.Address]int{a: 1, b: 2})
	if u || !reflect.DeepEqual(gs, []int{1, 2}) {
		t.Fatal(gs, u)
	}
	events := []assetEvent{{10, []int{1}, false, 1, 0}, {12, []int{1}, false, 0, 1}, {13, []int{1}, false, 1, 0}, {13, []int{2}, false, 1, 0}, {15, []int{2}, false, 0, 1}}
	stages := assetSegments(events)
	if len(stages) != 3 || stages[0].Before.Height != 9 || stages[0].After.Height != 12 || stages[0].Collections != 1 || !reflect.DeepEqual(stages[1].Groups, []int{1, 2}) {
		t.Fatal(stages)
	}
}
func TestAssetTimelineLive(t *testing.T) {
	date := os.Getenv("FUNBOT_ASSET_TEST_DAY")
	if date == "" {
		t.Skip("set a date for a read-only RPC audit")
	}
	seats, e := readAssetManifest(os.Getenv("FUNBOT_ASSET_MANIFEST"))
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	c := NewChain([]string{"https://rpc.anubispace.org", "https://rpc1.anubispace.org"}, mainnetChainID)
	h, e := assetHeader(ctx, c, nil)
	if e != nil {
		t.Fatal(e)
	}
	d, e := assetDay(ctx, c, date, h)
	if e != nil {
		t.Fatal(e)
	}
	v, e := findAssetStages(ctx, c, d, seats)
	if e != nil {
		t.Fatal(e)
	}
	t.Log("stages", len(v))
	for _, w := range v {
		t.Log(w.Groups, w.Unknown, w.Plays, w.Collections, w.Before.Height, w.After.Height)
	}
}
