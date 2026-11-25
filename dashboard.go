package main

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

var rewardContract = common.HexToAddress("0xff1e7170c041c0e37bf9b9ce0feed600f3deaaaa")

type statToken struct {
	ID     string
	Status int
	Amount string
}
type statAccount struct {
	Address, Label, Kind       string
	Group, Games               int
	Wallet, Available, Deposit string
	claimableJiazi             big.Int
	Mint                       []uint64
	Tokens                     []statToken
	Stipends, ActiveStipends   int
	Wins                       int
	Days                       map[string]matchDay
}
type statReport struct {
	Block, Timestamp, Period, Deadline uint64
	Accounts                           []statAccount
}
type statCache struct {
	sync.Mutex
	at      time.Time
	roster  []VaultMeta
	report  *statReport
	matches matchCache
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	s.stats.Lock()
	defer s.stats.Unlock()
	seats := s.ks.Meta()
	if !slices.Equal(seats, s.stats.roster) || time.Since(s.stats.at) > 45*time.Second || s.stats.report == nil {
		ctx, cancel := context.WithTimeout(r.Context(), 180*time.Second)
		defer cancel()
		report, err := scanStats(ctx, s.chain, seats, &s.stats.matches)
		if err != nil {
			s.fail(w, 503, fmt.Errorf("统计刷新失败：%w", err))
			return
		}
		s.stats.report = report
		s.stats.roster = seats
		s.stats.at = time.Now()
	}
	s.writeJSON(w, 200, s.stats.report)
}

// Only eth_call jobs are accepted; all calls use the same block number.
type statCall struct {
	to   common.Address
	data []byte
	read func([]*big.Int) error
}

func readStatCalls(ctx context.Context, c *Chain, tag string, jobs []statCall) error {
	for len(jobs) > 0 {
		n := min(len(jobs), 16)
		batch := jobs[:n]
		jobs = jobs[n:]
		data := make([]hexutil.Bytes, n)
		req := make([]rpc.BatchElem, n)
		for i, j := range batch {
			req[i] = rpc.BatchElem{Method: "eth_call", Args: []any{map[string]any{"to": j.to, "data": hexutil.Bytes(j.data)}, tag}, Result: &data[i]}
		}
		err := c.retry(ctx, func(ctx context.Context, cl *ethclient.Client) error { return cl.Client().BatchCallContext(ctx, req) })
		if err != nil {
			return err
		}
		for i, j := range batch {
			if req[i].Error != nil {
				return req[i].Error
			}
			raw := data[i]
			if len(raw) == 0 || len(raw)%32 != 0 {
				return errors.New("合约返回数据不完整")
			}
			words := make([]*big.Int, 0, len(raw)/32)
			for len(raw) > 0 {
				words = append(words, new(big.Int).SetBytes(raw[:32]))
				raw = raw[32:]
			}
			if err := j.read(words); err != nil {
				return err
			}
		}
	}
	return nil
}
func statHeader(ctx context.Context, c *Chain, number *big.Int) (height, ts uint64, hash common.Hash, err error) {
	err = c.retry(ctx, func(ctx context.Context, cl *ethclient.Client) error {
		h, e := cl.HeaderByNumber(ctx, number)
		if e == nil {
			height = h.Number.Uint64()
			ts = h.Time
			hash = h.Hash()
		}
		return e
	})
	return
}
func scanStats(ctx context.Context, c *Chain, seats []VaultMeta, matches *matchCache) (*statReport, error) {
	sorted := slices.Clone(seats)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Kind != sorted[j].Kind {
			return sorted[i].Kind == "master"
		}
		return sorted[i].Index < sorted[j].Index
	})
	height, ts, hash, err := statHeader(ctx, c, nil)
	if err != nil {
		return nil, err
	}
	return statsAt(ctx, c, sorted, height, ts, hash, matches)
}
func statsAt(ctx context.Context, c *Chain, sorted []VaultMeta, height, ts uint64, hash common.Hash, matches *matchCache) (*statReport, error) {
	report := &statReport{Block: height, Timestamp: ts, Accounts: make([]statAccount, len(sorted))}
	if len(sorted) == 0 {
		return report, nil
	}
	if sorted[0].Kind != "master" {
		return nil, errors.New("名册缺少主号")
	}
	master := common.HexToAddress(sorted[0].Address)
	tag := hexutil.EncodeUint64(height)
	deadline := func(period uint64) (uint64, error) {
		var value uint64
		err := readStatCalls(ctx, c, tag, []statCall{{rewardContract, mandateData("claimDeadline(uint256)", mandateInt(period)), func(w []*big.Int) error {
			if len(w) != 1 || !w[0].IsUint64() {
				return errors.New("福泽截止时间无效")
			}
			value = w[0].Uint64()
			return nil
		}}})
		return value, err
	}
	first, err := deadline(1)
	if err != nil {
		return nil, err
	}
	report.Period = 1
	if ts >= first {
		report.Period = 2 + (ts-first)/518400
	}
	report.Deadline, err = deadline(report.Period)
	if err != nil {
		return nil, err
	}
	var jobs, detail []statCall
	// Each page may enqueue the next page. Both the page and follow-up calls retain the block tag.
	var page func(*statAccount, bool, uint64) statCall
	page = func(a *statAccount, nft bool, offset uint64) statCall {
		addr := common.BytesToHash(common.HexToAddress(a.Address).Bytes())
		target, sig, width := Mainnet.Arena, "jiaziPage(address,uint256,uint256)", 6
		if nft {
			target, sig, width = mandateContract, "ownedPage(address,uint256,uint256)", 1
		}
		return statCall{target, mandateData(sig, addr, mandateInt(offset), mandateInt(32)), func(w []*big.Int) error {
			if len(w) < 3 || w[0].Cmp(big.NewInt(64)) != 0 || !w[1].IsUint64() || !w[2].IsUint64() {
				return errors.New("统计分页表头无效")
			}
			total, n := w[1].Uint64(), w[2].Uint64()
			if total < offset || n != min(uint64(32), total-offset) || len(w) != 3+int(n)*width {
				return errors.New("统计分页数量不符")
			}
			for i := 3; i < len(w); i += width {
				token := common.BigToHash(w[i])
				id := w[i].String()
				if nft {
					detail = append(detail, statCall{rewardContract, mandateData("rewardOf(uint256,uint256)", mandateInt(report.Period), token), func(v []*big.Int) error {
						if len(v) != 2 || !v[1].IsUint64() || v[1].Uint64() > 6 {
							return errors.New("福泽状态无效")
						}
						a.Tokens = append(a.Tokens, statToken{id, int(v[1].Uint64()), v[0].String()})
						return nil
					}})
				} else {
					a.Stipends++
					if matches == nil {
						a.claimableJiazi.Add(&a.claimableJiazi, w[i+5])
					}
					if w[i+2].Cmp(big.NewInt(60)) < 0 {
						a.ActiveStipends++
					}
				}
				if !nft && mandateMintEligible(w[i+2], w[i+4]) {
					detail = append(detail, statCall{mandateContract, mandateData("used(address,uint256)", addr, token), func(v []*big.Int) error {
						if len(v) != 1 || !v[0].IsUint64() || v[0].Uint64() > 1 || !token.Big().IsUint64() {
							return errors.New("铸令资格无效")
						}
						if v[0].Sign() == 0 {
							a.Mint = append(a.Mint, token.Big().Uint64())
						}
						return nil
					}})
				}
			}
			if offset+n < total {
				jobs = append(jobs, page(a, nft, offset+n))
			}
			return nil
		}}
	}
	for i, m := range sorted {
		a := &report.Accounts[i]
		*a = statAccount{Address: m.Address, Label: m.Label, Kind: m.Kind, Group: i/24 + 1}
		jobs = append(jobs, page(a, false, 0), page(a, true, 0))
		jobs = append(jobs, statCall{master, mandateData("pulse(address)", common.BytesToHash(common.HexToAddress(m.Address).Bytes())), func(w []*big.Int) error {
			if len(w) != 21 || !w[13].IsUint64() {
				return errors.New("打局状态读取失败，请检查主号 Squad 连接")
			}
			a.Games = int(w[13].Uint64())
			a.Wallet = w[0].String()
			a.Available = new(big.Int).Add(new(big.Int).Add(w[0], w[1]), w[3]).String()
			a.Deposit = w[2].String()
			return nil
		}})
	}
	for len(jobs) > 0 {
		current := jobs
		jobs = nil
		if err = readStatCalls(ctx, c, tag, current); err != nil {
			return nil, err
		}
	}
	if err = readStatCalls(ctx, c, tag, detail); err != nil {
		return nil, err
	}
	if matches == nil {
		for i := range report.Accounts {
			a := &report.Accounts[i]
			v, _ := new(big.Int).SetString(a.Available, 10)
			v.Add(v, &a.claimableJiazi)
			for _, t := range a.Tokens {
				if t.Status == 2 {
					n, _ := new(big.Int).SetString(t.Amount, 10)
					v.Add(v, n)
				}
			}
			a.Available = v.String()
		}
		return report, nil
	}
	next, err := readMatchHistory(ctx, c, report, hash, matches)
	if err != nil {
		return nil, err
	}
	_, _, check, err := statHeader(ctx, c, new(big.Int).SetUint64(height))
	if err != nil {
		return nil, err
	}
	if check != hash {
		return nil, errors.New("区块已重组，请刷新")
	}
	*matches = next
	return report, nil
}
