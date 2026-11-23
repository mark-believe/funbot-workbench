package main

import (
	"context"
	"fmt"
	"math/big"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

var collectOrder = []uint64{}
var alreadyOwned = []uint64{}

// Both balances and the owners of all 42 tokens must agree at one block.
func (s *Server) collectionPosition(ctx context.Context, block *big.Int) (int, error) {
	if block == nil {
		var height uint64
		err := s.chain.retry(ctx, func(ctx context.Context, c *ethclient.Client) error {
			var e error
			height, e = c.BlockNumber(ctx)
			return e
		})
		if err != nil {
			return 0, err
		}
		block = new(big.Int).SetUint64(height)
	}
	balance := func(a common.Address) (int, error) {
		raw, e := s.mandateRead(ctx, block, mandateData("balanceOf(address)", common.BytesToHash(a.Bytes())))
		if e != nil {
			return 0, e
		}
		n := new(big.Int).SetBytes(raw)
		if len(raw) != 32 || !n.IsUint64() || n.Uint64() > 42 {
			return 0, fmt.Errorf("持仓超出清单范围")
		}
		return int(n.Uint64()), nil
	}
	h, e := balance(mandateHolder)
	if e != nil {
		return 0, e
	}
	m, e := balance(mandateMaster)
	if e != nil {
		return 0, e
	}
	n := len(collectOrder) - h
	if n < 0 || m != len(alreadyOwned)+n {
		return 0, fmt.Errorf("持仓与清单不符：主号 %d，指定来源账号 %d", m, h)
	}
	if e = s.mandateOwners(ctx, block, alreadyOwned, mandateMaster); e != nil {
		return 0, e
	}
	if e = s.mandateOwners(ctx, block, collectOrder[:n], mandateMaster); e != nil {
		return 0, e
	}
	if e = s.mandateOwners(ctx, block, collectOrder[n:], mandateHolder); e != nil {
		return 0, e
	}
	return n, nil
}

func (s *Server) handleCollect37(w http.ResponseWriter, r *http.Request) {
	if !s.st.TryBusy("归集 37 枚盟主令") {
		s.fail(w, 409, fmt.Errorf("已有任务正在运行"))
		return
	}
	s.ks.Touch()
	go func() {
		defer s.st.Idle()
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
		defer cancel()
		if err := s.collectMandates(ctx); err != nil {
			s.st.Logf("bad", "归集停止：%v", err)
		}
	}()
	s.ok(w)
}

func (s *Server) collectMandates(ctx context.Context) error {
	if mandateMaster == (common.Address{}) || mandateHolder == (common.Address{}) || mandateSquad == (common.Address{}) || len(originalMandates) != 38 || len(collectOrder) != 37 || len(alreadyOwned) != 5 { return fmt.Errorf("请先配置本地专属地址和令牌清单") }
	sq, master, err := s.run.ready()
	if err != nil {
		return err
	}
	if master.Address != mandateMaster || sq.impl != mandateSquad || s.chain.chainID != 6714 {
		return fmt.Errorf("主号、Squad 或链不符")
	}
	if err = s.run.verifyAttached(ctx); err != nil {
		return err
	}
	for _, a := range []common.Address{mandateHolder, mandateMaster} {
		if err = sq.requireDelegated(ctx, a); err != nil {
			return err
		}
	}
	from := s.ks.Find(mandateHolder)
	if from == nil {
		return fmt.Errorf("请先解锁指定来源账号")
	}
	if err = s.chain.checkPending(ctx); err != nil {
		return err
	}
	start, err := s.collectionPosition(ctx, nil)
	if err != nil {
		return err
	}
	source := common.BytesToHash(mandateHolder.Bytes())
	dest := common.BytesToHash(mandateMaster.Bytes())
	topic := crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))
	var total, stage uint64
	for i := start; i < len(collectOrder); i++ {
		id := collectOrder[i]
		price := s.chain.GasPrice()
		if price.Cmp(big.NewInt(12000000000)) != 0 {
			return fmt.Errorf("Gas 价格偏离 12 Gwei")
		}
		if err = s.mandateOwners(ctx, nil, []uint64{id}, mandateHolder); err != nil {
			return err
		}
		data := mandateData("safeTransferFrom(address,address,uint256)", source, dest, mandateInt(id))
		var gas uint64
		err = s.chain.retry(ctx, func(ctx context.Context, c *ethclient.Client) error {
			var e error
			gas, e = c.EstimateGas(ctx, ethereum.CallMsg{From: mandateHolder, To: &mandateContract, GasPrice: price, Data: data})
			return e
		})
		if err != nil {
			return err
		}
		if gas == 0 || gas > 120000 {
			return fmt.Errorf("#%d 估算 %d Gas 超出预算范围", id, gas)
		}
		rec, e := s.chain.SendCall(ctx, from, mandateContract, data, min(gas+gas/5, 120000))
		if e != nil {
			return e
		}
		s.st.Logf("", "#%d · %s · 区块 %v · 实际 Gas %d · 状态 %d", id, rec.TxHash, rec.BlockNumber, rec.GasUsed, rec.Status)
		if rec.Status != 1 {
			return fmt.Errorf("#%d 交易回滚", id)
		}
		count := 0
		for _, l := range rec.Logs {
			if l.Address != mandateContract || len(l.Topics) == 0 || l.Topics[0] != topic {
				continue
			}
			if len(l.Topics) != 4 || l.Topics[1] != source || l.Topics[2] != dest || l.Topics[3] != mandateInt(id) {
				return fmt.Errorf("#%d 回执事件不符", id)
			}
			count++
		}
		if count != 1 {
			return fmt.Errorf("#%d 缺少唯一的转账事件", id)
		}
		if err = s.mandateOwners(ctx, rec.BlockNumber, []uint64{id}, mandateMaster); err != nil {
			return err
		}
		total += rec.GasUsed
		stage += rec.GasUsed
		n := i + 1
		if n == 1 || n == 13 || n == 25 || n == 37 {
			pos, e := s.collectionPosition(ctx, rec.BlockNumber)
			if e != nil {
				return e
			}
			if pos != n {
				return fmt.Errorf("归集进度与回执不符")
			}
			s.st.Logf("ok", "已核对 %d/37，主号 %d 枚，指定来源账号 %d 枚。本次运行本批实际 Gas %d，累计 %d", n, n+5, 37-n, stage, total)
			stage = 0
		}
	}
	s.st.Logf("ok", "清单核对完成：主号 42 枚，指定来源账号 为 0 枚。本次运行从第 %d 枚后继续，实际 Gas %d", start, total)
	return nil
}
