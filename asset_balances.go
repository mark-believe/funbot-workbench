package main

import (
	"context"
	"fmt"
	"math/big"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient"
)

type assetBlock struct {
	Height, Time uint64
	Hash         common.Hash
}

func assetHeader(ctx context.Context, c *Chain, n *big.Int) (assetBlock, error) {
	h, t, hash, err := statHeader(ctx, c, n)
	return assetBlock{h, t, hash}, err
}
func blockNum(h uint64) *big.Int { return new(big.Int).SetUint64(h) }

type assetAccount struct {
	Address, Label                                           string
	Group                                                    int
	Wallet, Arena, Jiazi, Lineage, Fuze, Occupied, Available string
}
type assetSheet struct {
	Block    assetBlock
	Accounts []assetAccount
}

func weiValue(s string) *big.Int {
	if s == "" {
		return new(big.Int)
	}
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		panic("invalid internal wei value")
	}
	return n
}

func (a *assetAccount) sum() *big.Int {
	n := new(big.Int)
	for _, v := range []string{a.Wallet, a.Arena, a.Jiazi, a.Lineage, a.Fuze} {
		n.Add(n, weiValue(v))
	}
	return n
}
func assetRoster(seats []VaultMeta) ([]VaultMeta, error) {
	seats = slices.Clone(seats)
	sort.SliceStable(seats, func(i, j int) bool {
		if seats[i].Kind != seats[j].Kind {
			return seats[i].Kind == "master"
		}
		return seats[i].Index < seats[j].Index
	})
	seen := map[common.Address]bool{}
	if len(seats) == 0 || seats[0].Kind != "master" {
		return nil, fmt.Errorf("请先载入含主号的账号名册")
	}
	for i, s := range seats {
		a := common.HexToAddress(s.Address)
		if !common.IsHexAddress(s.Address) || a == (common.Address{}) || seen[a] || (i > 0 && s.Kind == "master") {
			return nil, fmt.Errorf("账号名册有重复或无效地址")
		}
		seen[a] = true
	}
	return seats, nil
}

// Each account owns its callbacks. Workers never mutate another account's page or subtotal.
func readAssets(ctx context.Context, c *Chain, b assetBlock, seats []VaultMeta) (*assetSheet, error) {
	tag := hexutil.EncodeUint64(b.Height)
	var period uint64
	var deployed bool
	err := c.retry(ctx, func(ctx context.Context, cl *ethclient.Client) error {
		code, e := cl.CodeAt(ctx, rewardContract, blockNum(b.Height))
		deployed = len(code) > 0
		return e
	})
	if err != nil {
		return nil, err
	}
	if deployed {
		err = readStatCalls(ctx, c, tag, []statCall{{rewardContract, mandateData("claimDeadline(uint256)", mandateInt(1)), func(w []*big.Int) error {
			if len(w) != 1 || !w[0].IsUint64() || w[0].Sign() == 0 {
				return fmt.Errorf("福泽期次无效")
			}
			period = 1
			if first := w[0].Uint64(); b.Time >= first {
				period = 2 + (b.Time-first)/518400
			}
			return nil
		}}})
		if err != nil {
			return nil, err
		}
	}
	sheet := &assetSheet{b, make([]assetAccount, len(seats))}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	var once sync.Once
	var firstErr error
	// Interleave four independent chunks to bound pressure on the public RPC.
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var jobs []statCall
			for i := worker; i < len(seats); i += 4 {
				a := &sheet.Accounts[i]
				s := seats[i]
				*a = assetAccount{Address: s.Address, Label: s.Label, Group: i/24 + 1, Fuze: "0"}
				addr := common.BytesToHash(common.HexToAddress(s.Address).Bytes())
				var page func(bool, uint64) statCall
				page = func(nft bool, offset uint64) statCall {
					to, sig, width := Mainnet.Arena, "jiaziPage(address,uint256,uint256)", 6
					if nft {
						to, sig, width = mandateContract, "ownedPage(address,uint256,uint256)", 1
					}
					return statCall{to, mandateData(sig, addr, mandateInt(offset), mandateInt(32)), func(w []*big.Int) error {
						if len(w) < 3 || w[0].Cmp(big.NewInt(64)) != 0 || !w[1].IsUint64() || !w[2].IsUint64() {
							return fmt.Errorf("资产分页表头异常")
						}
						total, n := w[1].Uint64(), w[2].Uint64()
						if total < offset || n != min(uint64(32), total-offset) || len(w) != 3+int(n)*width {
							return fmt.Errorf("资产分页长度不符")
						}
						for j := 3; j < len(w); j += width {
							if !nft {
								a.Jiazi = new(big.Int).Add(weiValue(a.Jiazi), w[j+5]).String()
								continue
							}
							jobs = append(jobs, statCall{rewardContract, mandateData("rewardOf(uint256,uint256)", mandateInt(period), common.BigToHash(w[j])), func(v []*big.Int) error {
								if len(v) != 2 || !v[1].IsUint64() || v[1].Uint64() > 6 {
									return fmt.Errorf("福泽状态异常")
								}
								if v[1].Uint64() == 2 {
									a.Fuze = new(big.Int).Add(weiValue(a.Fuze), v[0]).String()
								}
								return nil
							}})
						}
						if offset+n < total {
							jobs = append(jobs, page(nft, offset+n))
						}
						return nil
					}}
				}
				jobs = append(jobs, statCall{Mainnet.Arena, mandateData("playerSummary(address)", addr), func(v []*big.Int) error {
					if len(v) != 16 || v[10].BitLen() > 1 {
						return fmt.Errorf("擂台资产摘要不完整")
					}
					a.Wallet = v[0].String()
					a.Arena = v[2].String()
					a.Occupied = v[3].String()
					a.Jiazi = v[9].String()
					if v[10].Sign() != 0 {
						a.Jiazi = "0"
						jobs = append(jobs, page(false, 0))
					}
					return nil
				}}, statCall{Mainnet.Arena, mandateData("lineagePending(address)", addr), func(v []*big.Int) error {
					if len(v) != 1 {
						return fmt.Errorf("师承余额不完整")
					}
					a.Lineage = v[0].String()
					return nil
				}})
				if deployed {
					jobs = append(jobs, page(true, 0))
				}
			}
			for len(jobs) > 0 {
				wave := jobs
				jobs = nil
				if e := readStatCalls(ctx, c, tag, wave); e != nil {
					once.Do(func() { firstErr = e; cancel() })
					return
				}
			}
		}()
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	check, err := assetHeader(ctx, c, blockNum(b.Height))
	if err != nil {
		return nil, err
	}
	if check.Hash != b.Hash {
		return nil, fmt.Errorf("区块重组，请刷新")
	}
	for i := range sheet.Accounts {
		a := &sheet.Accounts[i]
		a.Available = a.sum().String()
	}
	return sheet, nil
}

type assetCache struct {
	sync.Mutex
	roster    []VaultMeta
	windowMu  sync.Mutex
	sheets    map[common.Hash]*assetSheet
	order     []common.Hash
	timelines map[string]assetWindows
}

func (s *Server) handleAssetBalances(w http.ResponseWriter, r *http.Request) {
	h, err := strconv.ParseUint(r.URL.Query().Get("block"), 10, 64)
	hash := r.URL.Query().Get("hash")
	if err != nil || len(hash) != 66 {
		s.fail(w, 400, fmt.Errorf("无效的区块"))
		return
	}
	seats, err := assetRoster(s.ks.Meta())
	if err != nil {
		s.fail(w, 400, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	b, err := assetHeader(ctx, s.assetChain, blockNum(h))
	if err != nil || b.Hash.Hex() != hash {
		s.fail(w, 503, fmt.Errorf("无法核实资产区块 %d，请刷新", h))
		return
	}
	cache := &s.assets
	cache.Lock()
	defer cache.Unlock()
	if !slices.Equal(seats, cache.roster) {
		cache.sheets = map[common.Hash]*assetSheet{}
		cache.order = nil
		cache.roster = slices.Clone(seats)
	}
	sheet := cache.sheets[b.Hash]
	if sheet == nil {
		sheet, err = readAssets(ctx, s.assetChain, b, seats)
		if err != nil {
			s.fail(w, 503, fmt.Errorf("区块 %d 资产读取失败：%w", h, err))
			return
		}
		if len(cache.order) >= 100 {
			delete(cache.sheets, cache.order[0])
			cache.order = cache.order[1:]
		}
		cache.order = append(cache.order, b.Hash)
		cache.sheets[b.Hash] = sheet
	}
	s.writeJSON(w, 200, sheet)
}
