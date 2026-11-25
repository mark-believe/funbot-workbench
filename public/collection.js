"use strict";
(()=>{
 const scan=$("collectionPreview"),send=$("collectionConfirm"),status=$("collectionStatus"),rows=$("collectionRows");
 let preview;
 scan.onclick=async()=>{
  $("collectionDialog").showModal();
  preview=null;rows.replaceChildren();send.hidden=true;scan.disabled=true;
  status.textContent="正在同一区块核对子账号的铸令资格和令牌持仓…";
  try{
   const result=await api("/api/mandate/collection-plan");
   const items=result.plan.Items||[];
   const mints=result.plan.Mints||[];
   if(!items.length&&!mints.length){status.textContent="当前子账号没有可铸造或归集的令牌。";return}
   const sources=new Map();
   const detailFor=address=>{
    if(!sources.has(address))sources.set(address,{ids:[],indices:[]});
    return sources.get(address);
   };
   for(const item of items){
    detailFor(item.Source).ids.push("#"+item.ID);
   }
   for(const mint of mints){
    detailFor(mint.Source).indices.push(mint.Index);
   }
   status.innerHTML="待铸造 "+mints.length+" 枚，预计归集 "+(items.length+mints.length)+" 枚。目标主号 "+addressHTML(result.plan.Target)+"。按 12 Gwei 计算，铸造与转移的总 Gas 费用上限为 "+tokenAmount(result.maxFeeWei)+" DAI。";
   const table=document.createElement("table");table.className="match-table";table.innerHTML="<thead><tr><th>来源账户</th><th>待铸造 · 甲子索引</th><th>已有 · 令牌编号</th></tr></thead>";const tbody=table.createTBody();
   for(const [address,{ids,indices}] of sources){
    const tr=tbody.insertRow();tr.insertCell().innerHTML=addressHTML(address);
    tr.insertCell().textContent=indices.join("、")||"—";
    tr.insertCell().textContent=ids.join("、")||"—";
   }
   rows.append(table);preview=result;
   send.textContent=mints.length?"确认铸造并归集":"确认归集";
   send.hidden=demoMode;
   if(demoMode)status.append(" 当前为测试数据。");
  }catch(e){status.textContent="预检未完成 · "+e.message}
  finally{scan.disabled=false}
 };
 send.onclick=async()=>{
  if(!preview||demoMode)return;
  const p=preview;
  const mints=p.plan.Mints?.length||0, total=(p.plan.Items?.length||0)+mints;
  const action=mints?"先铸造 "+mints+" 枚，再归集共 "+total+" 枚":"归集 "+total+" 枚";
  if(!confirm("确认"+action+"到 "+p.plan.Target+"？Gas 费用上限 "+tokenAmount(p.maxFeeWei)+" DAI。"))return;
  preview=null;send.hidden=true;scan.disabled=true;
  try{await api("/api/mandate/collection-confirm",{Approval:p.approval});status.textContent="任务已提交，请在执行日志查看铸造、归集及链上回执。"}
  catch(e){status.textContent="提交失败 · "+e.message}
  finally{scan.disabled=false}
 };
})();
