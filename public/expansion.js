"use strict";
let expansionBook=null,readingExpansion=false,bookError="";
function renderExpansion(){
 if(!report)return;
 const groups=[...new Set(report.Accounts.map(a=>a.Group))];
 let reached=0,assessed=0,profitable=0;
 const lines=groups.map(g=>{
  const accounts=report.Accounts.filter(a=>a.Group===g);
  const amount=accounts.reduce((n,a)=>n+a.Stipends,0);
  const active=accounts.reduce((n,a)=>n+a.ActiveStipends,0);
  if(amount>=240)reached++;
  const fixed=expansionBook?.manifest?.groups?.[g]||[];
  const same=accounts.length===fixed.length&&accounts.every(a=>fixed.includes(a.Address.toLowerCase()));
  const record=same?expansionBook?.book?.groups?.[g]:null;
  const covered=record?.state==="可开一组"||record?.state?.startsWith("已回本");
  if(amount>=240 && record?.capital!=null && record?.profit!=null){assessed++;if(covered)profitable++;}
  const state=bookError?"账本读取失败":!expansionBook?"读取中":!same?"名册待复核":record?.state||"待核算";
  const firstOrEstimate=[record?.first && "首次 "+record.first,record?.eta && "预计 "+record.eta].filter(Boolean).join(" / ") || "—";
  const money=v=>v==null?"—":esc(v);
  return '<tr><th>第 '+g+' 组</th><td>'+amount+' / '+active+'</td><td>'+(amount>=240?"已达线":240-amount)+'</td><td class="'+(covered?'ex-ready':'')+'">'+esc(state)+'</td><td>'+money(record?.gap)+'</td><td>'+esc(firstOrEstimate)+'</td><td>'+money(record?.spare)+'</td><td>'+money(record?.fundingGap)+'</td><td>'+esc(record?.date||"—")+'</td></tr>';
 });
 setHTML("ex-groups",lines.join(""));
 put("ex-note",bookError||("按最新区块统计，"+reached+" 组达到 240 条观察线。其中有完整回本数据 "+assessed+" 组，已核实回本 "+profitable+" 组。资金列单位 fLGNS，按各组核算日显示。"));
}
async function readExpansion(){
 if(readingExpansion||document.hidden||tab!=="stats")return;
 readingExpansion=true;
 try{
  const data=await api("/api/expansion");
  if(data.book.error)throw Error(data.book.error);
  expansionBook=data;bookError="";
 }catch(e){expansionBook=null;bookError="核算表未能读取："+e.message;}
 finally{readingExpansion=false;renderExpansion();}
}
$("ex-read").onclick=readExpansion;
$("ex-open").onclick=async()=>{try{await api("/api/expansion/open",{});}catch(e){put("ex-note",e.message);}};
$("ds-refresh").addEventListener("click",readExpansion);
document.querySelector('.tab[data-tab="stats"]').addEventListener("click",readExpansion);
window.addEventListener("load",readExpansion);
document.addEventListener("visibilitychange",readExpansion);
setInterval(readExpansion,60000);
