package main

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestCollectionApproval(t *testing.T) {
	var a collectionApproval
	p := collectionPlan{Target: common.HexToAddress("0x1"), Squad: common.HexToAddress("0x2"), Items: []collectionItem{{common.HexToAddress("0x3"), "123"}}}
	d := p.digest()
	token, e := a.issue(d)
	if e != nil {
		t.Fatal(e)
	}
	p.Items[0].ID = "124"
	if p.digest() == d {
		t.Fatal("substitution")
	}
	if _, ok := a.consume("invalid"); ok {
		t.Fatal("invalid approval")
	}
	if got, ok := a.consume(token); !ok || got != d {
		t.Fatal("valid approval rejected")
	}
	if _, ok := a.consume(token); ok {
		t.Fatal("replayed approval")
	}
	token, e = a.issue(d)
	if e != nil {
		t.Fatal(e)
	}
	a.until = time.Now().Add(-time.Second)
	if _, ok := a.consume(token); ok {
		t.Fatal("expired approval")
	}
}

func TestCollectionMintOnlyPlan(t *testing.T) {
	source := common.HexToAddress("0x3")
	var plan collectionPlan
	var next []statCall
	page := collectionMintPage(source, 0, &plan, &next)
	values := []int64{64, 3, 3, 0, 0, 60, 0, 60, 0, 4, 0, 60, 0, 59, 0, 7, 0, 60, 0, 60, 0}
	var words []*big.Int
	for _, value := range values {
		words = append(words, big.NewInt(value))
	}
	if err := page.read(words); err != nil {
		t.Fatal(err)
	}
	if len(next) != 2 {
		t.Fatalf("got %d eligibility checks, want 2", len(next))
	}
	if err := next[0].read([]*big.Int{big.NewInt(0)}); err != nil {
		t.Fatal(err)
	}
	if err := next[1].read([]*big.Int{big.NewInt(1)}); err != nil {
		t.Fatal(err)
	}
	if len(plan.Mints) != 1 || plan.Mints[0] != (collectionMint{source, "0"}) {
		t.Fatalf("unexpected mints: %#v", plan.Mints)
	}
	if got := plan.maxFee().String(); got != "11969064000000000" {
		t.Fatalf("mint-only fee should include a subsequent transfer, got %s", got)
	}
	digest := plan.digest()
	plan.Mints[0].Index = "1"
	if plan.digest() == digest {
		t.Fatal("approval did not bind the mint index")
	}
}

func TestCollectionMintID(t *testing.T) {
	source := mandateInt(3)
	event := &types.Log{Address: mandateContract, Topics: []common.Hash{
		crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)")), {}, source, mandateInt(42),
	}}
	rec := &types.Receipt{Status: types.ReceiptStatusSuccessful, Logs: []*types.Log{event}}
	if id, err := collectionMintID(rec, source); err != nil || id != 42 {
		t.Fatalf("unexpected mint result: id %d, err %v", id, err)
	}
	if _, err := collectionMintID(rec, mandateInt(4)); err == nil {
		t.Fatal("accepted a mint to a different account")
	}
	rec.Logs = append(rec.Logs, event)
	if _, err := collectionMintID(rec, source); err == nil {
		t.Fatal("accepted more than one mint event")
	}
}
