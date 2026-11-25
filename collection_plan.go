package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

const (
	collectionGasCap     uint64 = 120000
	collectionMintGasCap uint64 = 877422
	collectionGasPrice   int64  = 12000000000
)

type collectionItem struct {
	Source common.Address
	ID     string
}
type collectionMint struct {
	Source common.Address
	Index  string
}
type collectionPlan struct {
	Target, Squad common.Address
	Items         []collectionItem
	Mints         []collectionMint
}

func (p collectionPlan) maxFee() *big.Int {
	gas := new(big.Int).SetUint64(collectionGasCap*uint64(len(p.Items)+len(p.Mints)) + collectionMintGasCap*uint64(len(p.Mints)))
	return gas.Mul(gas, big.NewInt(collectionGasPrice))
}

func (p collectionPlan) digest() string {
	raw, _ := json.Marshal(p)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (s *Server) collectionPlan(ctx context.Context) (collectionPlan, error) {
	var plan collectionPlan
	sq, master, e := s.run.ready()
	if e != nil {
		return plan, e
	}
	if s.chain.chainID != mainnetChainID {
		return plan, fmt.Errorf("网络不符")
	}
	if e = s.run.verifyAttached(ctx); e != nil {
		return plan, e
	}
	plan.Target, plan.Squad = master.Address, sq.impl
	h, _, hash, e := statHeader(ctx, s.chain, nil)
	if e != nil {
		return plan, e
	}
	tag := hexutil.EncodeUint64(h)
	seats, e := assetRoster(s.ks.Meta())
	if e != nil {
		return plan, e
	}
	var calls []statCall
	var seen = map[uint64]bool{}
	// Enumerate at one height. Validate each page against the contract's total.
	for _, seat := range seats {
		addr := common.HexToAddress(seat.Address)
		if addr == plan.Target {
			continue
		}
		word := common.BytesToHash(addr.Bytes())
		calls = append(calls, collectionMintPage(addr, 0, &plan, &calls))
		var page func(uint64) statCall
		var total uint64
		page = func(offset uint64) statCall {
			return statCall{mandateContract, mandateData("ownedPage(address,uint256,uint256)", word, mandateInt(offset), mandateInt(32)), func(w []*big.Int) error {
				if len(w) < 3 || w[0].Cmp(big.NewInt(64)) != 0 || !w[1].IsUint64() || !w[2].IsUint64() {
					return fmt.Errorf("令牌分页数据无效")
				}
				if offset == 0 {
					total = w[1].Uint64()
				}
				n := w[2].Uint64()
				if w[1].Uint64() != total || offset > total || n != min(uint64(32), total-offset) || len(w) != 3+int(n) {
					return fmt.Errorf("令牌清单不完整")
				}
				for _, id := range w[3:] {
					if !id.IsUint64() || seen[id.Uint64()] {
						return fmt.Errorf("令牌编号重复或不受支持")
					}
					seen[id.Uint64()] = true
					plan.Items = append(plan.Items, collectionItem{addr, id.String()})
				}
				if offset+n < total {
					calls = append(calls, page(offset+n))
				}
				return nil
			}}
		}
		calls = append(calls, page(0))
	}
	for len(calls) > 0 {
		wave := calls
		calls = nil
		if e = readStatCalls(ctx, s.chain, tag, wave); e != nil {
			return plan, e
		}
	}
	sort.Slice(plan.Mints, func(i, j int) bool {
		a, b := plan.Mints[i], plan.Mints[j]
		if a.Source != b.Source {
			return a.Source.Hex() < b.Source.Hex()
		}
		if len(a.Index) != len(b.Index) {
			return len(a.Index) < len(b.Index)
		}
		return a.Index < b.Index
	})
	for i := 1; i < len(plan.Mints); i++ {
		if plan.Mints[i] == plan.Mints[i-1] {
			return plan, fmt.Errorf("铸令资格清单有重复索引")
		}
	}
	sort.Slice(plan.Items, func(i, j int) bool {
		a, b := plan.Items[i], plan.Items[j]
		if a.Source != b.Source {
			return a.Source.Hex() < b.Source.Hex()
		}
		if len(a.ID) != len(b.ID) {
			return len(a.ID) < len(b.ID)
		}
		return a.ID < b.ID
	})
	for _, item := range plan.Items {
		id, err := strconv.ParseUint(item.ID, 10, 64)
		if err != nil {
			return plan, err
		}
		calls = append(calls, statCall{mandateContract, mandateData("ownerOf(uint256)", mandateInt(id)), func(w []*big.Int) error {
			if len(w) != 1 || w[0].Cmp(new(big.Int).SetBytes(item.Source.Bytes())) != 0 {
				return fmt.Errorf("#%s 持有人与清单不符", item.ID)
			}
			return nil
		}})
	}
	if e = readStatCalls(ctx, s.chain, tag, calls); e != nil {
		return plan, e
	}
	_, _, check, e := statHeader(ctx, s.chain, blockNum(h))
	if e != nil {
		return plan, e
	}
	if check != hash {
		return plan, fmt.Errorf("区块已重组，请重新预检")
	}
	return plan, nil
}

func collectionMintPage(addr common.Address, offset uint64, plan *collectionPlan, next *[]statCall) statCall {
	word := common.BytesToHash(addr.Bytes())
	return statCall{Mainnet.Arena, mandateData("jiaziPage(address,uint256,uint256)", word, mandateInt(offset), mandateInt(32)), func(w []*big.Int) error {
		const width = 6
		if len(w) < 3 || w[0].Cmp(big.NewInt(64)) != 0 || !w[1].IsUint64() || !w[2].IsUint64() {
			return fmt.Errorf("铸令资格分页数据无效")
		}
		total, n := w[1].Uint64(), w[2].Uint64()
		if offset > total || n != min(uint64(32), total-offset) || len(w) != 3+int(n)*width {
			return fmt.Errorf("铸令资格清单不完整")
		}
		for i := 3; i < len(w); i += width {
			if !mandateMintEligible(w[i+2], w[i+4]) {
				continue
			}
			index := w[i]
			if !index.IsUint64() {
				return fmt.Errorf("不支持的铸令索引")
			}
			*next = append(*next, statCall{mandateContract, mandateData("used(address,uint256)", word, common.BigToHash(index)), func(v []*big.Int) error {
				if len(v) != 1 || !v[0].IsUint64() || v[0].Uint64() > 1 {
					return fmt.Errorf("铸令资格状态无效")
				}
				if v[0].Sign() == 0 {
					plan.Mints = append(plan.Mints, collectionMint{addr, index.String()})
				}
				return nil
			}})
		}
		if offset+n < total {
			*next = append(*next, collectionMintPage(addr, offset+n, plan, next))
		}
		return nil
	}}
}

type collectionApproval struct {
	sync.Mutex
	token, digest string
	until         time.Time
}

func (a *collectionApproval) issue(digest string) (string, error) {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	a.Lock()
	defer a.Unlock()
	a.token = hex.EncodeToString(b[:])
	a.digest = digest
	a.until = time.Now().Add(10 * time.Minute)
	return a.token, nil
}
func (a *collectionApproval) consume(token string) (string, bool) {
	a.Lock()
	defer a.Unlock()
	if token == "" || a.token != token || time.Now().After(a.until) {
		return "", false
	}
	a.token = ""
	return a.digest, true
}
func (s *Server) previewCollection(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	p, e := s.collectionPlan(ctx)
	if e != nil {
		s.fail(w, 400, e)
		return
	}
	token, e := s.collection.issue(p.digest())
	if e != nil {
		s.fail(w, 500, e)
		return
	}
	s.writeJSON(w, 200, map[string]any{"plan": p, "approval": token, "maxFeeWei": p.maxFee().String()})
}

func (s *Server) confirmCollection(w http.ResponseWriter, r *http.Request) {
	var req struct{ Approval string }
	if e := s.body(r, &req); e != nil {
		s.fail(w, 400, e)
		return
	}
	if len(req.Approval) != 48 {
		s.fail(w, 400, fmt.Errorf("请先核对归集清单"))
		return
	}
	if !s.st.TryBusy("按已确认清单铸造并归集令牌") {
		s.fail(w, 409, fmt.Errorf("已有任务正在运行"))
		return
	}
	digest, ok := s.collection.consume(req.Approval)
	if !ok {
		s.st.Idle()
		s.fail(w, 409, fmt.Errorf("预检已过期或已使用，请重新预检"))
		return
	}
	s.ks.Touch()
	go func() {
		defer s.st.Idle()
		ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
		defer cancel()
		if e := s.executeCollection(ctx, digest); e != nil {
			s.st.Logf("bad", "归集停止：%v", e)
		}
	}()
	s.ok(w)
}

func (s *Server) executeCollection(ctx context.Context, digest string) error {
	p, e := s.collectionPlan(ctx)
	if e != nil {
		return e
	}
	if len(p.Items)+len(p.Mints) == 0 || p.digest() != digest {
		return fmt.Errorf("持仓、铸令资格、目标或 Squad 已变化，请重新预检")
	}
	for _, mint := range p.Mints {
		id, err := s.mintForCollection(ctx, p, mint)
		if err != nil {
			return fmt.Errorf("铸令失败（%s，甲子索引 %s）：%w", mint.Source.Hex(), mint.Index, err)
		}
		p.Items = append(p.Items, collectionItem{mint.Source, strconv.FormatUint(id, 10)})
	}
	for _, item := range p.Items {
		id, e := strconv.ParseUint(item.ID, 10, 64)
		if e != nil {
			return e
		}
		if e = s.mandateOwners(ctx, nil, []uint64{id}, item.Source); e != nil {
			return e
		}
		source, dest := common.BytesToHash(item.Source.Bytes()), common.BytesToHash(p.Target.Bytes())
		data := mandateData("safeTransferFrom(address,address,uint256)", source, dest, mandateInt(id))
		rec, e := s.sendCollectionCall(ctx, p, item.Source, data, collectionGasCap)
		if e != nil {
			return e
		}
		s.st.Logf("", "#%d · %s · Gas %d", id, rec.TxHash, rec.GasUsed)
		if rec.Status != 1 {
			return fmt.Errorf("#%d 回滚", id)
		}
		transfers := 0
		topic := crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))
		for _, l := range rec.Logs {
			if l.Address == mandateContract && len(l.Topics) > 0 && l.Topics[0] == topic {
				if len(l.Topics) != 4 || l.Topics[1] != source || l.Topics[2] != dest || l.Topics[3] != mandateInt(id) {
					return fmt.Errorf("回执与确认清单不符")
				}
				transfers++
			}
		}
		if transfers != 1 {
			return fmt.Errorf("缺少唯一转账回执")
		}
		if e = s.mandateOwners(ctx, rec.BlockNumber, []uint64{id}, p.Target); e != nil {
			return e
		}
	}
	s.st.Logf("ok", "已铸造 %d 枚，已归集并核验 %d 枚令牌", len(p.Mints), len(p.Items))
	return nil
}

func (s *Server) mintForCollection(ctx context.Context, p collectionPlan, mint collectionMint) (uint64, error) {
	index, err := strconv.ParseUint(mint.Index, 10, 64)
	if err != nil {
		return 0, err
	}
	source := common.BytesToHash(mint.Source.Bytes())
	used, err := s.mandateRead(ctx, nil, mandateData("used(address,uint256)", source, mandateInt(index)))
	if err != nil {
		return 0, err
	}
	if len(used) != 32 || common.BytesToHash(used) != (common.Hash{}) {
		return 0, fmt.Errorf("铸令资格已变化，请重新预检")
	}
	data := mandateData("mint(uint256[])", mandateInt(32), mandateInt(1), mandateInt(index))
	rec, err := s.sendCollectionCall(ctx, p, mint.Source, data, collectionMintGasCap)
	if err != nil {
		return 0, err
	}
	s.st.Logf("", "铸令 · 甲子索引 %d · %s · Gas %d", index, rec.TxHash, rec.GasUsed)
	id, err := collectionMintID(rec, source)
	if err != nil {
		return 0, err
	}
	if err = s.mandateOwners(ctx, rec.BlockNumber, []uint64{id}, mint.Source); err != nil {
		return 0, err
	}
	s.st.Logf("ok", "铸令已核验 · #%d · %s", id, mint.Source.Hex())
	return id, nil
}

func collectionMintID(rec *types.Receipt, source common.Hash) (uint64, error) {
	if rec.Status != types.ReceiptStatusSuccessful {
		return 0, fmt.Errorf("铸令交易回滚：%s", rec.TxHash)
	}
	var id uint64
	found := false
	topic := crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))
	for _, l := range rec.Logs {
		if l.Address != mandateContract || len(l.Topics) == 0 || l.Topics[0] != topic {
			continue
		}
		if found || len(l.Topics) != 4 || l.Topics[1] != (common.Hash{}) || l.Topics[2] != source || !l.Topics[3].Big().IsUint64() {
			return 0, fmt.Errorf("铸令回执与确认清单不符：%s", rec.TxHash)
		}
		id, found = l.Topics[3].Big().Uint64(), true
	}
	if !found {
		return 0, fmt.Errorf("缺少铸令回执事件：%s", rec.TxHash)
	}
	return id, nil
}

func (s *Server) sendCollectionCall(ctx context.Context, p collectionPlan, source common.Address, data []byte, maxGas uint64) (*types.Receipt, error) {
	sq, master, err := s.run.ready()
	if err != nil {
		return nil, err
	}
	if master.Address != p.Target || sq.impl != p.Squad {
		return nil, fmt.Errorf("账号或合约已变化")
	}
	if err = s.run.verifyAttached(ctx); err != nil {
		return nil, err
	}
	for _, addr := range []common.Address{p.Target, source} {
		if err = sq.requireDelegated(ctx, addr); err != nil {
			return nil, err
		}
	}
	if err = s.chain.checkPending(ctx); err != nil {
		return nil, err
	}
	from := s.ks.Find(source)
	if from == nil {
		return nil, fmt.Errorf("来源账户尚未解锁")
	}
	price := s.chain.GasPrice()
	if price.Cmp(big.NewInt(collectionGasPrice)) != 0 {
		return nil, fmt.Errorf("Gas 价格偏离确认预算 12 Gwei")
	}
	var gas uint64
	err = s.chain.retry(ctx, func(ctx context.Context, c *ethclient.Client) error {
		var err error
		gas, err = c.EstimateGas(ctx, ethereum.CallMsg{From: source, To: &mandateContract, GasPrice: price, Data: data})
		return err
	})
	if err != nil {
		return nil, err
	}
	if gas == 0 || gas > maxGas {
		return nil, fmt.Errorf("估算 %d Gas 超出本笔预算 %d Gas", gas, maxGas)
	}
	return s.chain.SendCall(ctx, from, mandateContract, data, min(maxGas, gas+gas/5))
}
