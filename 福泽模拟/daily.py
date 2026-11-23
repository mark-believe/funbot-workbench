"""Daily read-only audit. Uses public addresses; has no signing or sending API."""
import json
import os
import subprocess
import sys
import tempfile
from datetime import datetime, timedelta, timezone
from pathlib import Path

MASTER = "0x0000000000000000000000000000000000000000"
NFT = "0xfffe850fd170625a8f816a731b9b278cc161aaaa"
REWARD = "0xff1e7170c041c0e37bf9b9ce0feed600f3deaaaa"
TOKEN = "0xffffbe730e2cde54d8928d36f547e41deb66aaaa"
IDS = ()
FIRST = 0
TRANSFER = "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"
ROOT = Path(__file__).resolve().parent
CN = timezone(timedelta(hours=8))
ALLOWED = {"eth_call","eth_getBlockByNumber","eth_chainId","eth_gasPrice","eth_simulateV1",
           "eth_getLogs","eth_getTransactionByHash","eth_getTransactionReceipt"}
def require(condition, message):
    if not condition:
        raise ValueError(message)

def batch(calls):
    results = []
    for offset in range(0,len(calls),16):
        req = []
        for i,(method,params) in enumerate(calls[offset:offset+16]):
            require(method in ALLOWED,"RPC method not permitted")
            req.append(dict(jsonrpc="2.0",id=i,method=method,params=params))
        raw = subprocess.run(["curl.exe","-fsS","--retry","2","--max-time","40","-H","Content-Type: application/json",
                              "--data-binary","@-","https://rpc.anubispace.org"],input=json.dumps(req).encode(),capture_output=True,check=True)
        answers = {r["id"]:r for r in json.loads(raw.stdout)}
        for i in range(len(req)):
            answer = answers[i]
            require("result" in answer and answer["result"] is not None,str(answer.get("error","RPC missing result")))
            results.append(answer["result"])
    return results

def rpc(method,*params):
    return batch([(method,list(params))])[0]

def word(n):
    return format(n,"064x")

def claim_data(period,ids):
    return "0xb1b66652"+word(period)+word(64)+word(len(ids))+"".join(map(word,ids))

def call(to,data,tag):
    return ("eth_call",[{"to":to,"data":data},tag])

def words(value):
    require(value.startswith("0x") and len(value)>2 and (len(value)-2)%64==0,"Invalid contract response")
    return [int(value[i:i+64],16) for i in range(2,len(value),64)]

def check_simulated(calls,amount,eligible):
    require(len(calls)==len(eligible)+3 and all(int(r["status"],16)==1 for r in calls),"领取模拟执行失败")
    before,after = (words(calls[i]["returnData"])[0] for i in (0,2))
    require(after-before==amount,"模拟到账与可领金额不一致")
    for r in calls[3:]:
        result=words(r["returnData"])
        require(len(result)==2 and result[1]==3,"领取后状态未变为已领取")
    gas=int(calls[1]["gasUsed"],16)
    require(gas>0,"模拟 Gas 无效")
    return gas

def confirmed(start,end,previous):
    ledger=list(previous)
    topic="0x"+word(int(MASTER,16))
    # Discover successful direct claims through fLGNS transfers to the master.
    for lo in range(start,end+1,2000):
        flt={"address":TOKEN,"topics":[TRANSFER,None,topic],"fromBlock":hex(lo),"toBlock":hex(min(end,lo+1999))}
        logs=rpc("eth_getLogs",flt)
        totals={}
        for event in logs:
            h=event["transactionHash"]
            totals[h]=totals.get(h,0)+int(event["data"],16)
        txs=batch([("eth_getTransactionByHash",[h]) for h in totals])
        for tx in txs:
            data=tx["input"]
            if tx["from"].lower()!=MASTER or (tx["to"] or "").lower()!=REWARD or not data.startswith("0xb1b66652"):
                continue
            period,ptr,n,*ids=words("0x"+data[10:])
            require(ptr==64 and n==len(ids),"领取交易参数无效")
            if not set(ids).intersection(IDS):
                continue
            require(set(ids)<=set(IDS),"发现混合清单领取，请核对后归属手续费")
            rec=rpc("eth_getTransactionReceipt",tx["hash"])
            require(rec["status"]=="0x1" and rec["blockHash"]==tx["blockHash"],"领取回执状态不一致")
            gas=int(rec["gasUsed"],16)
            ledger.append(dict(hash=tx["hash"],block=int(rec["blockNumber"],16),period=period,ids=ids,
                               amount=str(totals[tx["hash"]]),gas=gas,fee=str(gas*int(rec["effectiveGasPrice"],16))))
    return ledger

def run():
    require(FIRST>0 and int(MASTER,16)!=0 and len(set(IDS))==42,"清单数量不符")
    require(int(rpc("eth_chainId"),16)==6714,"网络不符")
    head=rpc("eth_getBlockByNumber","latest",False)
    tag=head["number"];height=int(tag,16);ts=int(head["timestamp"],16)
    first,nft,token=batch([call(REWARD,"0xa32093c0"+word(1),tag),call(REWARD,"0x7c0b8de2",tag),call(REWARD,"0x82bfefc8",tag)])
    require(int(nft,16)==int(NFT,16) and int(token,16)==int(TOKEN,16),"福泽合约配置不符")
    period=1 if ts<int(first,16) else 2+(ts-int(first,16))//518400
    owners=batch([call(NFT,"0x6352211e"+word(i),tag) for i in IDS])
    held=[i for i,o in zip(IDS,owners) if int(o,16)==int(MASTER,16)]
    reads=[call(REWARD,"0x3bf2a83e"+word(period)+word(i),tag) for i in held]
    eligible=[]; amount=0
    for i,raw in zip(held,batch(reads)):
        value,status=words(raw)
        require(status in range(7),"未知福泽状态")
        if status==2:
            eligible.append(i);amount+=value
    gas=fee=0
    if eligible:
        balance={"from":MASTER,"to":TOKEN,"data":"0x70a08231"+word(int(MASTER,16))}
        claim={"from":MASTER,"to":REWARD,"data":claim_data(period,eligible)}
        checks=[{"from":MASTER,"to":REWARD,"data":"0x3bf2a83e"+word(period)+word(i)} for i in eligible]
        sim=rpc("eth_simulateV1",{"blockStateCalls":[{"calls":[balance,claim,balance,*checks]}],"validation":False},tag)
        gas=check_simulated(sim[0]["calls"],amount,eligible)
        fee=gas*int(rpc("eth_gasPrice"),16)
    path=ROOT/"latest.json"
    start=FIRST;previous=[]
    if path.exists():
        old=json.loads(path.read_text("utf-8"))
        require(height>=old["block"],"节点高度滞后")
        oldhead=rpc("eth_getBlockByNumber",hex(old["block"]),False)
        if oldhead["hash"]==old["hash"]:
            start=old["block"]+1;previous=old["ledger"]
    ledger=confirmed(start,height,previous)
    require(rpc("eth_getBlockByNumber",tag,False)["hash"]==head["hash"],"区块重组，请重试")
    return dict(block=height,hash=head["hash"],timestamp=ts,period=period,held=len(held),missing=sorted(set(IDS)-set(held)),
                eligible=eligible,amount=str(amount),gas=gas,fee=str(fee),ledger=ledger,
                actualAmount=str(sum(int(r["amount"]) for r in ledger)),actualGas=sum(r["gas"] for r in ledger),
                actualFee=str(sum(int(r["fee"]) for r in ledger)))

def save(name,value):
    with tempfile.NamedTemporaryFile(dir=ROOT,delete=False,mode="w",encoding="utf-8") as f:
        json.dump(value,f,ensure_ascii=False)
    os.replace(f.name,ROOT/name)

if __name__=="__main__":
    try:
        result=run()
        save("latest.json",result)
        save(datetime.fromtimestamp(result["timestamp"],CN).strftime("%Y-%m-%d.json"),result)
        save("status.json",{"ok":True})
        print(json.dumps({k:result[k] for k in ("block","period","held","eligible","amount","gas","actualAmount","actualGas")}))
    except Exception as exc:
        save("status.json",{"ok":False,"at":datetime.now(CN).isoformat(timespec="minutes"),"error":str(exc)[:400]})
        raise
