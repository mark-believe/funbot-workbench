package main

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"
)

// Opt-in live check against public addresses only. Never opens the keystore.
func TestLiveStats(t *testing.T) {
	path := os.Getenv("FUNBOT_STATS_TEST_ROSTER")
	if path == "" {
		t.Skip("supply a public roster to run live verification")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var roster struct{ Seats []VaultMeta }
	if err = json.Unmarshal(b, &roster); err != nil {
		t.Fatal(err)
	}
	c := NewChain([]string{"https://rpc.anubispace.org", "https://rpc1.anubispace.org"}, 6714)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	cache := &matchCache{}
	report, err := scanStats(ctx, c, roster.Seats, cache)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Accounts) != len(roster.Seats) || len(report.Accounts) == 0 {
		t.Fatal("account coverage")
	}
	ids := map[string]bool{}
	groups := map[int]bool{}
	games, mint, claim := 0, 0, 0
	for _, a := range report.Accounts {
		groups[a.Group] = true
		games += a.Games
		mint += len(a.Mint)
		for _, tok := range a.Tokens {
			if ids[tok.ID] {
				t.Fatal("duplicate token", tok.ID)
			}
			ids[tok.ID] = true
			if tok.Status == 2 {
				claim++
			}
		}
	}
	day := time.Unix(int64(report.Timestamp), 0).In(chinaTime).Format("2006-01-02")
	groupWins := map[int]int{}
	groupNew := map[int]int{}
	for _, a := range report.Accounts {
		wins := 0
		for _, d := range a.Days {
			wins += d.Wins
			if d.Wins > d.Settled || d.Settled > d.Games {
				t.Fatal("invalid counts")
			}
		}
		if wins != a.Wins {
			t.Fatal("daily wins do not sum to lifetime")
		}
		groupWins[a.Group] += wins
		groupNew[a.Group] += a.Days[day].Wins
	}
	t.Logf("day %s, cumulative %v, new %v", day, groupWins, groupNew)
	warm := *report
	warm.Accounts = append([]statAccount(nil), report.Accounts...)
	for i := range warm.Accounts {
		warm.Accounts[i].Wins = 0
		warm.Accounts[i].Days = nil
	}
	_, err = readMatchHistory(ctx, c, &warm, cache.Hash, cache)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(warm.Accounts, report.Accounts) {
		t.Fatal("cold/warm history mismatch")
	}
	t.Logf("block %d, accounts %d, groups %d, games %d, mint %d, NFTs %d, master NFTs %d, claimable %d", report.Block, len(report.Accounts), len(groups), games, mint, len(ids), len(report.Accounts[0].Tokens), claim)
}
