package main

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

type matchDay struct{ Games, Settled, Wins int }
type matchPrefix struct {
	Offset uint64
	Days   map[string]matchDay
}
type matchCache struct {
	Height  uint64
	Hash    common.Hash
	Players map[string]matchPrefix
}

var chinaTime = time.FixedZone("CST", 8*3600)

func countMatch(days map[string]matchDay, day string, settled, won bool) {
	d := days[day]
	d.Games++
	if settled {
		d.Settled++
		if won {
			d.Wins++
		}
	}
	days[day] = d
}

// Re-read from the first unsettled entry. Only the immutable settled prefix
// is reused, and only while the previous snapshot is on the canonical chain.
func readMatchHistory(ctx context.Context, c *Chain, r *statReport, hash common.Hash, old *matchCache) (matchCache, error) {
	next := matchCache{r.Block, hash, make(map[string]matchPrefix, len(r.Accounts))}
	var cached map[string]matchPrefix
	if old.Players != nil && old.Height <= r.Block {
		_, _, previous, err := statHeader(ctx, c, new(big.Int).SetUint64(old.Height))
		if err != nil {
			return next, err
		}
		if previous == old.Hash {
			cached = old.Players
		}
	}
	var jobs []statCall
	for i := range r.Accounts {
		a := &r.Accounts[i]
		prefix := cached[a.Address]
		days := make(map[string]matchDay, len(prefix.Days))
		a.Days = make(map[string]matchDay, len(prefix.Days))
		for day, d := range prefix.Days {
			days[day] = d
			a.Days[day] = d
			a.Wins += d.Wins
		}
		prefix.Days = days
		var enqueue func(uint64)
		enqueue = func(offset uint64) {
			data := mandateData("playerHistory(address,uint256,uint256)", common.BytesToHash(common.HexToAddress(a.Address).Bytes()), mandateInt(offset), mandateInt(64))
			jobs = append(jobs, statCall{Mainnet.Arena, data, func(w []*big.Int) error {
				if len(w) < 3 || w[0].Cmp(big.NewInt(64)) != 0 || !w[1].IsUint64() || !w[2].IsUint64() {
					return fmt.Errorf("账号 %s 战绩表头无效", a.Address)
				}
				total, n := w[1].Uint64(), w[2].Uint64()
				if total < offset || n != min(uint64(64), total-offset) || len(w) != 3+int(n)*8 {
					return fmt.Errorf("账号 %s 战绩分页不完整", a.Address)
				}
				for j := uint64(0); j < n; j++ {
					row := w[3+j*8:]
					if !row[1].IsUint64() || row[1].Sign() == 0 || row[1].Uint64() > r.Timestamp || row[3].BitLen() > 1 || row[4].BitLen() > 1 {
						return fmt.Errorf("账号 %s 战绩值异常", a.Address)
					}
					day := time.Unix(row[1].Int64(), 0).In(chinaTime).Format("2006-01-02")
					settled, won := row[3].Sign() != 0, row[4].Sign() != 0
					countMatch(a.Days, day, settled, won)
					if settled && won {
						a.Wins++
					}
					if settled && prefix.Offset == offset+j {
						countMatch(prefix.Days, day, true, won)
						prefix.Offset++
					}
				}
				if offset+n < total {
					enqueue(offset + n)
				} else {
					next.Players[a.Address] = prefix
				}
				return nil
			}})
		}
		enqueue(prefix.Offset)
	}
	for len(jobs) > 0 {
		current := jobs
		jobs = nil
		if err := readStatCalls(ctx, c, hexutil.EncodeUint64(r.Block), current); err != nil {
			return next, fmt.Errorf("读取盟主战绩：%w", err)
		}
	}
	return next, nil
}
