"use strict";
(()=>{
 const scan=$("collectionPreview"),send=$("collectionConfirm"),status=$("collectionStatus"),rows=$("collectionRows");
 let preview;
 scan.onclick=async()=>{
  preview=null;rows.replaceChildren();send.hidden=true;scan.disabled=true;
  status.textContent="正在同一区块核对本机名册内的令牌持仓…";
  try{
   const result=await api("/api/mandate/collection-plan");
   const items=result.plan.Items||[];
   if(!items.length){status.textContent="当前无可归集令牌。已排除目标主号自身持仓。";return}
   const sources=new Map();
   for(const item of items){
    if(!sources.has(item.Source))sources.set(item.Source,[]);
    sources.get(item.Source).push("#"+item.ID);
   }
   status.textContent=sources.size+" 个来源账户，共 "+items.length+" 枚。目标主号 "+result.plan.Target+"。按 12 Gwei，每笔上限 120,000 Gas，合计费用上限 "+tokenAmount(result.maxFeeWei)+" DAI。";
   const table=document.createElement("table");table.className="match-table";
   for(const [address,ids] of sources){
    const tr=table.insertRow();tr.insertCell().textContent=address;tr.insertCell().textContent=ids.length+" 枚";tr.insertCell().textContent=ids.join("、");
   }
   rows.append(table);preview=result;send.hidden=false;
  }catch(e){status.textContent="预检未完成 · "+e.message}
  finally{scan.disabled=false}
 };
 send.onclick=async()=>{
  if(!preview)return;
  const p=preview;
  if(!confirm("确认把清单内 "+p.plan.Items.length+" 枚令牌逐笔转入 "+p.plan.Target+"？费用上限 "+tokenAmount(p.maxFeeWei)+" DAI。"))return;
  preview=null;send.hidden=true;scan.disabled=true;
  try{await api("/api/mandate/collection-confirm",{Approval:p.approval});status.textContent="已提交核验，请在执行日志查看每笔回执和最终结果。"}
  catch(e){status.textContent="提交失败 · "+e.message}
  finally{scan.disabled=false}
 };
})();
