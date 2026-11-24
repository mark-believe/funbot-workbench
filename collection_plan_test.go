package main

import (
	"github.com/ethereum/go-ethereum/common"
	"testing"
	"time"
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
