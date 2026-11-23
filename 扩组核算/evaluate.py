"""Read independent group snapshots; never write back to Excel or the vault."""
import json
import math
import os
import subprocess
import sys
from datetime import date,datetime,timedelta,timezone
from decimal import Decimal
from pathlib import Path
from openpyxl import load_workbook

ROOT=Path(__file__).resolve().parent
CN=timezone(timedelta(hours=8))
FIELDS=("capital","profit","cash","reserve","budget")
BOOK=ROOT/"分组核算.xlsx"

def require(ok,msg):
    if not ok: raise ValueError(msg)

def evaluate(records):
    if not records: return {"state":"待核算"}
    records=sorted(records,key=lambda r:r["date"])
    first=None
    for r in records:
        if None not in (r["capital"],r["profit"]) and r["profit"]>=r["capital"]:
            first=r["date"];break
    r=records[-1];capital,profit,cash,reserve,budget=(r[k] for k in FIELDS)
    known=capital is not None and profit is not None
    gap=max(Decimal(0),capital-profit) if known else None
    spare=cash-reserve if None not in (cash,reserve) else None
    funded=known and gap==0
    state="待核算" if not known else "未回本" if gap else "已回本"
    if funded and None not in (spare,budget): state="可开一组" if spare>=budget else "已回本·资金不足"
    eta=None;span=None
    if known and gap>0:
        # Use the longest available 7–30 day interval at unchanged capital.
        for older in reversed(records[:-1]):
            days=(r["date"]-older["date"]).days
            if days>30 or older["capital"]!=capital:break
            if days>=7 and older["profit"] is not None:
                gain=profit-older["profit"];eta=None;span=days
                if gain>0:
                    remaining=math.ceil(gap*days/gain)
                    if remaining<(date.max-r["date"]).days:eta=r["date"]+timedelta(days=remaining)
    return {**r,"state":state,"gap":gap,"spare":spare,"first":first,"eta":eta,"span":span,
            "fundingGap":max(Decimal(0),budget-spare) if None not in (budget,spare) else None}

def read_book(path=BOOK):
    wb=load_workbook(path,read_only=True,data_only=False)
    records={};seen=set()
    try:
        sheet=wb["分组核算"]
        require(tuple(c.value for c in sheet[7][:7])==("核算日期","组号","累计投入","累计已实现净收益","可调配余额","老组预留","开一组预算"),"第7行核算表列名已改变")
        for line,values in enumerate(sheet.iter_rows(min_row=8,max_col=8,values_only=True),8):
            when,group,*rest=values
            if when is None and all(v is None for v in rest):continue
            require(isinstance(group,int) and group>0,f"第{line}行组号无效")
            try:
                day=when.date() if isinstance(when,datetime) else date.fromisoformat(str(when))
            except ValueError:raise ValueError(f"第{line}行需要填写核算日期")
            require(day<=datetime.now(CN).date(),f"第{line}行日期晚于今天")
            require((group,day) not in seen,f"第{line}行同组同日重复")
            seen.add((group,day));r={"date":day}
            for key,v in zip(FIELDS,rest):
                if v is None:r[key]=None;continue
                require(isinstance(v,(float,int)) and not isinstance(v,bool) and math.isfinite(v),f"第{line}行金额须填写已核实的数值")
                require(key=="profit" or v>=0,f"第{line}行只有净收益可以为负")
                require(key not in ("capital","budget") or v>0,f"第{line}行投入和预算须大于零")
                r[key]=Decimal(str(v))
            records.setdefault(str(group),[]).append(r)
    finally:wb.close()
    return {g:evaluate(rs) for g,rs in records.items()}

def rpc(req):
    data=[dict(jsonrpc="2.0",id=i,method=m,params=p) for i,(m,p) in enumerate(req)]
    raw=subprocess.run(["curl.exe","-fsS","--retry","2","--max-time","30","-H","Content-Type: application/json","--data-binary","@-","https://rpc.anubispace.org"],input=json.dumps(data).encode(),capture_output=True,check=True)
    out={r["id"]:r for r in json.loads(raw.stdout)}
    return [out[i]["result"] for i in range(len(req))]

def observations():
    manifest=json.loads((ROOT/"groups.json").read_text("utf8"))
    chain,block=rpc([("eth_chainId",[]),("eth_getBlockByNumber",["latest",False])])
    require(int(chain,16)==6714,"网络不符")
    arena="0xff060bab41e16ebf0efe4fad454404fd5f75aaaa"
    counts={}
    for group,addresses in manifest["groups"].items():
        req=[("eth_call",[{"to":arena,"data":manifest["selector"]+a[2:].zfill(64)},block["number"]]) for a in addresses]
        counts[group]=sum(int(n,16) for i in range(0,len(req),16) for n in rpc(req[i:i+16]))
    require(rpc([("eth_getBlockByNumber",[block["number"],False])])[0]["hash"]==block["hash"],"区块已重组")
    return counts

def notify():
    ledger=read_book(); counts=observations()
    statefile=ROOT/"已提醒.json"
    prior=json.loads(statefile.read_text("utf8")) if statefile.exists() else {}
    current={};messages=[]
    for g,n in counts.items():
        r=ledger.get(g,{});state=r.get("state","待核算");met=n>=250
        current[g]=[state,met]
        prev=prior.get(g,["待核算",False])
        if state!=prev[0] and (state.startswith("已回本") or state=="可开一组" or prev[0] not in ("待核算","未回本")):
            messages.append(f"第{g}组 {state}（核算日 {r.get('date','—')}）")
        if met and not prev[1]:
            messages.append(f"第{g}组累计{n}条甲子，达到观察线；财务状态：{state}")
    if messages:
        env={**os.environ,"FUNBOT_LOCAL_NOTICE":"；".join(messages)}
        subprocess.run(["powershell.exe","-NoProfile","-NonInteractive","-WindowStyle","Hidden","-Command",
            "Add-Type -AssemblyName System.Windows.Forms; Add-Type -AssemblyName System.Drawing; $n=New-Object Windows.Forms.NotifyIcon; try { $n.Icon=[Drawing.SystemIcons]::Information; $n.Visible=$true; $n.ShowBalloonTip(15000,'共建工作台 · 扩组核算',$env:FUNBOT_LOCAL_NOTICE,[Windows.Forms.ToolTipIcon]::Info); Start-Sleep -Seconds 16 } finally {$n.Dispose()}"],env=env,check=True)
    temp=statefile.with_suffix(".tmp");temp.write_text(json.dumps(current,ensure_ascii=False),"utf8");os.replace(temp,statefile)
    print(json.dumps({"counts":counts,"notifications":len(messages)}))

if __name__=="__main__":
    if sys.argv[1:]==["--notify"]:notify()
    elif sys.argv[1:]==["--read"]:
        try: print(json.dumps({"groups":read_book()},default=str,ensure_ascii=True))
        except Exception as e: print(json.dumps({"error":str(e)},ensure_ascii=True))
    else:raise SystemExit("expected --read or --notify")
