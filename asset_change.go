package main

import (
	"context"
	"fmt"
	"math/big"
	"net/http"
	"sort"
	"strconv"
	"time"
)

// lastBefore finds the closing block before a Beijing calendar boundary.
func lastBefore(ctx context.Context, c *Chain, head, ts uint64) (uint64, error) {
	lo, hi := uint64(0), head+1
	for lo < hi {
		mid := lo + (hi-lo)/2
		_, t, _, e := statHeader(ctx, c, new(big.Int).SetUint64(mid))
		if e != nil {
			return 0, e
		}
		if t < ts {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo == 0 {
		return 0, fmt.Errorf("日期早于创世区块")
	}
	return lo - 1, nil
}
func (s *Server) assetChange(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Minute)
	defer cancel()
	seats := s.ks.Meta()
	if len(seats) == 0 {
		s.fail(w, 400, fmt.Errorf("请先导入账号名册"))
		return
	}
	sort.SliceStable(seats, func(i, j int) bool {
		if seats[i].Kind != seats[j].Kind {
			return seats[i].Kind == "master"
		}
		return seats[i].Index < seats[j].Index
	})
	seen := map[string]bool{}
	for _, a := range seats {
		if seen[a.Address] {
			s.fail(w, 400, fmt.Errorf("名册有重复地址"))
			return
		}
		seen[a.Address] = true
	}
	head, now, _, err := statHeader(ctx, s.chain, nil)
	if err != nil {
		s.fail(w, 503, err)
		return
	}
	q := r.URL.Query()
	var start, end uint64
	if q.Get("first") != "" || q.Get("last") != "" {
		f, e := strconv.ParseUint(q.Get("first"), 10, 64)
		l, e2 := strconv.ParseUint(q.Get("last"), 10, 64)
		if e != nil || e2 != nil || f == 0 || l < f || l > head {
			s.fail(w, 400, fmt.Errorf("请核对首末交易区块"))
			return
		}
		start, end = f-1, l
	} else {
		d, e := time.ParseInLocation("2006-01-02", q.Get("date"), chinaTime)
		if e != nil || d.Unix() < 0 || uint64(d.Unix()) > now {
			s.fail(w, 400, fmt.Errorf("请选择有效日期"))
			return
		}
		start, err = lastBefore(ctx, s.chain, head, uint64(d.Unix()))
		end = head
		if err == nil && uint64(d.AddDate(0, 0, 1).Unix()) <= now {
			end, err = lastBefore(ctx, s.chain, head, uint64(d.AddDate(0, 0, 1).Unix()))
		}
	}
	if err != nil {
		s.fail(w, 503, err)
		return
	}
	var reports []*statReport
	var hashes []string
	for _, b := range []uint64{start, end} {
		_, t, h, e := statHeader(ctx, s.chain, new(big.Int).SetUint64(b))
		if e != nil {
			s.fail(w, 503, e)
			return
		}
		snap, e := statsAt(ctx, s.chain, seats, b, t, h, nil)
		if e != nil {
			s.fail(w, 503, fmt.Errorf("区块 %d 的历史资产读取失败：%w", b, e))
			return
		}
		reports = append(reports, snap)
		hashes = append(hashes, h.Hex())
	}
	for i, b := range []uint64{start, end} {
		_, _, h, e := statHeader(ctx, s.chain, new(big.Int).SetUint64(b))
		if e != nil || h.Hex() != hashes[i] {
			s.fail(w, 503, fmt.Errorf("区块复核失败，请重查"))
			return
		}
	}
	s.writeJSON(w, 200, reports)
}
