package main

import (
	"context"
	"fmt"
	"math/big"
	"net/http"
	"slices"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

var mandateContract = common.HexToAddress("0xfffe850fd170625a8f816a731b9b278cc161aaaa")
var mandateMaster = common.HexToAddress("0x0000000000000000000000000000000000000000")
var mandateHolder = common.HexToAddress("0x0000000000000000000000000000000000000000")
var mandateSquad = common.HexToAddress("0x0000000000000000000000000000000000000000")
const personalGiftID uint64 = 0

var originalMandates = []uint64{}

func mandateData(signature string, words ...common.Hash) []byte {
	data := crypto.Keccak256([]byte(signature))[:4:4]
	for _, w := range words {
		data = append(data, w[:]...)
	}
	return data
}
func mandateInt(i uint64) common.Hash { return common.BigToHash(new(big.Int).SetUint64(i)) }
func (s *Server) mandateRead(ctx context.Context, block *big.Int, data []byte) ([]byte, error) {
	var out []byte
	err := s.chain.retry(ctx, func(ctx context.Context, c *ethclient.Client) error {
		var err error
		out, err = c.CallContract(ctx, ethereum.CallMsg{To: &mandateContract, Data: data}, block)
		return err
	})
	return out, err
}
func (s *Server) mandateOwners(ctx context.Context, block *big.Int, ids []uint64, want common.Address) error {
	for _, id := range ids {
		b, err := s.mandateRead(ctx, block, mandateData("ownerOf(uint256)", mandateInt(id)))
		if err != nil {
			return err
		}
		if len(b) != 32 || common.BytesToAddress(b) != want {
			return fmt.Errorf("令 #%d 持有人核验失败", id)
		}
	}
	return nil
}
func (s *Server) handleMandate(mint bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := s.busy("盟主令操作", func(ctx context.Context) error { return s.executeMandate(ctx, mint) })
		if err != nil {
			s.fail(w, 400, err)
			return
		}
		s.ok(w)
	}
}
func (s *Server) executeMandate(ctx context.Context, mint bool) error {
	if mandateMaster == (common.Address{}) || mandateHolder == (common.Address{}) || mandateSquad == (common.Address{}) || len(originalMandates) != 38 || len(collectOrder) != 37 || len(alreadyOwned) != 5 { return fmt.Errorf("请先配置本地专属地址和令牌清单") }
	sq, master, err := s.run.ready()
	if err != nil {
		return err
	}
	if master.Address != mandateMaster || sq.impl != mandateSquad || s.chain.chainID != 6714 || s.chain.GasPrice().Cmp(big.NewInt(12000000000)) != 0 {
		return fmt.Errorf("交易账号、网络、Squad 或 Gas 价格不符")
	}
	if err = s.run.verifyAttached(ctx); err != nil {
		return err
	}
	for _, a := range []common.Address{mandateMaster, mandateHolder} {
		if err = sq.requireDelegated(ctx, a); err != nil {
			return err
		}
	}
	masterWord := common.BytesToHash(mandateMaster.Bytes())
	sourceWord := common.BytesToHash(mandateHolder.Bytes())
	for i := uint64(0); i < 4; i++ {
		raw, err := s.mandateRead(ctx, nil, mandateData("used(address,uint256)", masterWord, mandateInt(i)))
		if err != nil {
			return err
		}
		expected := uint64(1)
		if mint {
			expected = 0
		}
		if len(raw) != 32 || common.BytesToHash(raw) != mandateInt(expected) {
			return fmt.Errorf("铸令资格 %d 状态不符，停止提交", i)
		}
	}
	if err = s.mandateOwners(ctx, nil, originalMandates, mandateHolder); err != nil {
		return err
	}
	from := master
	data := mandateData("mint(uint256[])", mandateInt(32), mandateInt(4), mandateInt(0), mandateInt(1), mandateInt(2), mandateInt(3))
	maxGas := uint64(877422)
	if !mint {
		from = s.ks.Find(mandateHolder)
		if from == nil {
			return fmt.Errorf("指定来源账号 未解锁")
		}
		data = mandateData("safeTransferFrom(address,address,uint256)", sourceWord, masterWord, mandateInt(personalGiftID))
		maxGas = 132522
	}
	var gas uint64
	err = s.chain.retry(ctx, func(ctx context.Context, c *ethclient.Client) error {
		var e error
		gas, e = c.EstimateGas(ctx, ethereum.CallMsg{From: from.Address, To: &mandateContract, Data: data, GasPrice: s.chain.GasPrice()})
		return e
	})
	if err != nil {
		return err
	}
	if gas == 0 || gas > maxGas {
		return fmt.Errorf("节点估算 %d Gas，本笔授权上限 %d Gas，未广播", gas, maxGas)
	}
	gasLimit := min(gas+gas/5, maxGas)
	receipt, err := s.chain.SendCall(ctx, from, mandateContract, data, gasLimit)
	if err != nil {
		return err
	}
	s.st.Logf("", "盟主令交易 %s · Gas %d · 区块 %s", receipt.TxHash.Hex(), receipt.GasUsed, receipt.BlockNumber)
	if receipt.Status != 1 {
		return fmt.Errorf("交易回滚：%s", receipt.TxHash)
	}
	topic := crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))
	expectedSource := sourceWord
	if mint {
		expectedSource = common.Hash{}
	}
	var ids []uint64
	for _, l := range receipt.Logs {
		if l.Address != mandateContract || len(l.Topics) != 4 || l.Topics[0] != topic {
			continue
		}
		if l.Topics[1] != expectedSource || l.Topics[2] != masterWord || !l.Topics[3].Big().IsUint64() {
			return fmt.Errorf("Transfer 事件与预期不符：%s", receipt.TxHash)
		}
		ids = append(ids, l.Topics[3].Big().Uint64())
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	if mint && len(ids) != 4 || !mint && !slices.Equal(ids, []uint64{personalGiftID}) {
		return fmt.Errorf("令编号核对失败：%v", ids)
	}
	if err = s.mandateOwners(ctx, receipt.BlockNumber, ids, mandateMaster); err != nil {
		return err
	}
	remaining := slices.Clone(originalMandates)
	if !mint {
		remaining = slices.DeleteFunc(remaining, func(id uint64) bool { return id == personalGiftID })
	}
	if err = s.mandateOwners(ctx, receipt.BlockNumber, remaining, mandateHolder); err != nil {
		return err
	}
	s.st.Logf("ok", "已核对：主号收到 %v，指定来源账号 原持仓剩余 %d 枚", ids, len(remaining))
	return nil
}
