package main

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

type assetWindow struct {
	Groups             []int
	Unknown            bool
	Plays, Collections int
	Before, After      assetBlock
}
type assetWindows struct {
	Day        string
	Start, End assetBlock
	Closed     bool
	Stages     []assetWindow
}

var ledgerABI = func() abi.ABI {
	a, e := abi.JSON(strings.NewReader(squadABI))
	if e != nil {
		panic(e)
	}
	return a
}()

func assetCut(ctx context.Context, c *Chain, top assetBlock, target uint64) (assetBlock, error) {
	if top.Time < target {
		return top, nil
	}
	hi := top
	span := max(uint64(16), (top.Time-target+1)*2)
	var lo assetBlock
	for {
		h := hi.Height - min(span, hi.Height)
		var e error
		lo, e = assetHeader(ctx, c, blockNum(h))
		if e != nil {
			return lo, e
		}
		if lo.Time < target {
			break
		}
		if h == 0 {
			return lo, fmt.Errorf("所选日期早于链上记录")
		}
		hi = lo
		span *= 2
	}
	for hi.Height-lo.Height > 1 {
		step := (target - lo.Time) * (hi.Height - lo.Height) / (hi.Time - lo.Time)
		h := lo.Height + max(uint64(1), min(step, hi.Height-lo.Height-1))
		mid, e := assetHeader(ctx, c, blockNum(h))
		if e != nil {
			return mid, e
		}
		if mid.Time < target {
			lo = mid
		} else {
			hi = mid
		}
	}
	return lo, nil
}
func assetDay(ctx context.Context, c *Chain, date string, head assetBlock) (assetWindows, error) {
	var d assetWindows
	start, err := time.ParseInLocation("2006-01-02", date, chinaTime)
	if err != nil || start.Unix() < 0 || start.Unix() > int64(head.Time) {
		return d, fmt.Errorf("请选择已有链上记录的北京时间日期")
	}
	d.Day = date
	d.End = head
	d.Closed = start.AddDate(0, 0, 1).Unix() <= int64(head.Time)
	if d.Closed {
		d.End, err = assetCut(ctx, c, head, uint64(start.AddDate(0, 0, 1).Unix()))
		if err != nil {
			return d, err
		}
	}
	d.Start, err = assetCut(ctx, c, d.End, uint64(start.Unix()))
	return d, err
}

type assetTx struct {
	Input     hexutil.Bytes
	To        *common.Address
	BlockHash common.Hash
}

func txGroups(raw []byte, byAddress map[common.Address]int) ([]int, bool) {
	if len(raw) < 4 {
		return nil, true
	}
	method, err := ledgerABI.MethodById(raw[:4])
	if err != nil {
		return nil, true
	}
	vals, err := method.Inputs.Unpack(raw[4:])
	if err != nil || len(vals) == 0 {
		return nil, true
	}
	var addrs []common.Address
	switch v := vals[0].(type) {
	case []common.Address:
		addrs = v
	case common.Address:
		addrs = []common.Address{v}
	default:
		return nil, true
	}
	gs := map[int]bool{}
	unknown := len(addrs) == 0
	for _, addr := range addrs {
		g := byAddress[addr]
		if g == 0 {
			unknown = true
		} else {
			gs[g] = true
		}
	}
	keys := make([]int, 0, len(gs))
	for g := range gs {
		keys = append(keys, g)
	}
	sort.Ints(keys)
	return keys, unknown
}

type assetEvent struct {
	height          uint64
	groups          []int
	unknown         bool
	plays, collects int
}

func assetSegments(events []assetEvent) []assetWindow {
	var blocks []assetEvent
	for _, e := range events {
		if len(blocks) > 0 && blocks[len(blocks)-1].height == e.height {
			v := &blocks[len(blocks)-1]
			gs := append(slices.Clone(v.groups), e.groups...)
			sort.Ints(gs)
			v.groups = slices.Compact(gs)
			v.unknown = v.unknown || e.unknown
			v.plays += e.plays
			v.collects += e.collects
		} else {
			blocks = append(blocks, e)
		}
	}
	var stages []assetWindow
	for _, e := range blocks {
		var v *assetWindow
		if len(stages) > 0 {
			v = &stages[len(stages)-1]
		}
		if v == nil || v.Unknown || e.unknown || !slices.Equal(v.Groups, e.groups) {
			stages = append(stages, assetWindow{Groups: e.groups, Unknown: e.unknown, Before: assetBlock{Height: e.height - 1}})
			v = &stages[len(stages)-1]
		}
		v.Plays += e.plays
		v.Collections += e.collects
		v.After.Height = e.height
	}
	return stages
}
func assetLogs(ctx context.Context, c *Chain, master common.Address, from, to uint64) ([]types.Log, error) {
	play, harvest := ledgerABI.Events["RoundPlayed"].ID, ledgerABI.Events["Harvested"].ID
	var logs []types.Log
	for first := from; first <= to; {
		last := min(to, first+1999)
		q := ethereum.FilterQuery{FromBlock: blockNum(first), ToBlock: blockNum(last), Addresses: []common.Address{master}, Topics: [][]common.Hash{{play, harvest}}}
		err := c.retry(ctx, func(ctx context.Context, cl *ethclient.Client) error {
			ls, e := cl.FilterLogs(ctx, q)
			if e == nil {
				logs = append(logs, ls...)
			}
			return e
		})
		if err != nil {
			return nil, err
		}
		first = last + 1
	}
	return logs, nil
}
func findAssetStages(ctx context.Context, c *Chain, d assetWindows, seats []VaultMeta) ([]assetWindow, error) {
	master := common.HexToAddress(seats[0].Address)
	groups := map[common.Address]int{}
	for i, s := range seats {
		groups[common.HexToAddress(s.Address)] = i/24 + 1
	}
	play := ledgerABI.Events["RoundPlayed"].ID
	logs, err := assetLogs(ctx, c, master, d.Start.Height+1, d.End.Height)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(logs, func(i, j int) bool {
		if logs[i].BlockNumber == logs[j].BlockNumber {
			return logs[i].Index < logs[j].Index
		}
		return logs[i].BlockNumber < logs[j].BlockNumber
	})
	var events []assetEvent
	for pos := 0; pos < len(logs); pos += 16 {
		page := logs[pos:min(pos+16, len(logs))]
		txs := make([]*assetTx, len(page))
		req := make([]rpc.BatchElem, len(page))
		for i, l := range page {
			req[i] = rpc.BatchElem{Method: "eth_getTransactionByHash", Args: []any{l.TxHash}, Result: &txs[i]}
		}
		err := c.retry(ctx, func(ctx context.Context, cl *ethclient.Client) error { return cl.Client().BatchCallContext(ctx, req) })
		if err != nil {
			return nil, err
		}
		for i, l := range page {
			tx := txs[i]
			if req[i].Error != nil || tx == nil || l.Removed || l.BlockHash != tx.BlockHash || len(l.Topics) == 0 {
				return nil, fmt.Errorf("无法核实打局或归集交易")
			}
			var gs []int
			unk := true
			if tx.To != nil && *tx.To == master {
				gs, unk = txGroups(tx.Input, groups)
			}
			ev := assetEvent{height: l.BlockNumber, groups: gs, unknown: unk}
			if l.Topics[0] == play {
				ev.plays = 1
			} else {
				ev.collects = 1
			}
			events = append(events, ev)
		}
	}
	stages := assetSegments(events)
	for i := range stages {
		var err error
		v := &stages[i]
		v.Before, err = assetHeader(ctx, c, blockNum(v.Before.Height))
		if err != nil {
			return nil, err
		}
		v.After, err = assetHeader(ctx, c, blockNum(v.After.Height))
		if err != nil {
			return nil, err
		}
	}
	return stages, nil
}
func (s *Server) handleAssetWindows(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	date := r.URL.Query().Get("date")
	seats, err := assetRoster(s.ks.Meta())
	if err != nil {
		s.fail(w, 400, err)
		return
	}
	head, err := assetHeader(ctx, s.assetChain, nil)
	if err != nil {
		s.fail(w, 503, err)
		return
	}
	mode := r.URL.Query().Get("mode")
	key := fmt.Sprint(date, "/", mode, "/", seats)
	cache := &s.assets
	cache.windowMu.Lock()
	defer cache.windowMu.Unlock()
	d, ok := cache.timelines[key]
	prior := d
	if ok && (d.Closed || head.Time >= d.End.Time && head.Time-d.End.Time < 55) {
		b, e := assetHeader(ctx, s.assetChain, blockNum(d.End.Height))
		if e == nil && b.Hash == d.End.Hash {
			s.writeJSON(w, 200, d)
			return
		}
	}
	d, err = assetDay(ctx, s.assetChain, date, head)
	if err == nil && mode == "stages" {
		reuse := false
		if ok && prior.Start.Hash == d.Start.Hash && prior.End.Height <= d.End.Height {
			check, e := assetHeader(ctx, s.assetChain, blockNum(prior.End.Height))
			if e == nil && check.Hash == prior.End.Hash {
				ls, e := assetLogs(ctx, s.assetChain, common.HexToAddress(seats[0].Address), prior.End.Height+1, d.End.Height)
				reuse = e == nil && len(ls) == 0
			}
		}
		if reuse {
			d.Stages = prior.Stages
		} else {
			d.Stages, err = findAssetStages(ctx, s.assetChain, d, seats)
		}
	}
	if err == nil {
		var b assetBlock
		b, err = assetHeader(ctx, s.assetChain, blockNum(d.End.Height))
		if err == nil && b.Hash != d.End.Hash {
			err = fmt.Errorf("区块已重组，请刷新")
		}
	}
	if err != nil {
		s.fail(w, 503, fmt.Errorf("读取资产时段失败：%w", err))
		return
	}
	if len(cache.timelines) >= 40 || cache.timelines == nil {
		cache.timelines = map[string]assetWindows{}
	}
	cache.timelines[key] = d
	s.writeJSON(w, 200, d)
}
