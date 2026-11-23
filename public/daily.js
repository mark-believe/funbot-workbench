"use strict";
function showDailyReport(data) {
  const r=data.latest;
  const warning=data.status?.ok===false?"上次检查失败 · "+data.status.at+" · "+data.status.error+"。":"";
  if(!r){
    put("df-status",warning+(data.available?"等待首次检查":"此模拟任务仅绑定原主号"));
    return;
  }
  const stale=Date.now()/1000-r.timestamp>86400;
  put("df-status",warning+(stale?"上次成功快照已超过一天 · ":"")+"第 "+r.period+" 期 · 持有 "+r.held+"/42 枚 · 可领 "+r.eligible.length+" 枚 · "+chainTime(r.timestamp)+"（北京时间）");
  put("df-amount",tokenAmount(r.amount)+" fLGNS");
  put("df-gas",r.gas.toLocaleString("en-US")+" Gas");
  put("df-actual",tokenAmount(r.actualAmount)+" fLGNS");
  put("df-paid",r.actualGas.toLocaleString("en-US")+" Gas");
  put("df-scope","每日 09:00（北京时间）检查。区块 "+r.block.toLocaleString("en-US")+"。可领编号："+(r.eligible.map(n=>"#"+n).join("、")||"无")+"。本次模拟手续费 "+tokenAmount(r.fee)+" DAI。实际金额与费用统计自区块 配置的起始区块 起，由主号直接领取清单内令牌的成功交易，累计实付 "+tokenAmount(r.actualFee)+" DAI。"+(r.missing.length?"已移出主号："+r.missing.join("、"):""));
  setHTML("df-ledger",r.ledger.length?r.ledger.slice().reverse().map(tx=>'<p>第 '+esc(tx.period)+' 期 · '+tokenAmount(tx.amount)+' fLGNS · '+Number(tx.gas).toLocaleString("en-US")+' Gas · '+tokenAmount(tx.fee)+' DAI<br><code>'+esc(tx.hash)+'</code></p>').join(""):"尚无符合范围的领取回执");
}
let dailyLoading=false;
async function loadDailyReport(){
  if(dailyLoading||document.hidden||tab!=="stats")return;
  dailyLoading=true;
  try{showDailyReport(await api("/api/daily-fuze"));}
  catch(e){put("df-status","读取失败 · "+e.message);}
  finally{dailyLoading=false;}
}
$("assetDaily").onclick=()=>{showTab("stats");$("daily-fuze").scrollIntoView({block:"center"});};
$("ds-refresh").addEventListener("click",loadDailyReport);
document.querySelector('.tab[data-tab="stats"]').addEventListener("click",loadDailyReport);
window.addEventListener("load",loadDailyReport);
document.addEventListener("visibilitychange",loadDailyReport);
setInterval(loadDailyReport,60000);
document.querySelector(".settings-nav").addEventListener("click",e=>{
  const b=e.target.closest("button[data-pane]");
  if(!b)return;
  $("setupBody").dataset.pane=b.dataset.pane;
  for(const item of b.parentElement.querySelectorAll("button")){
    item.setAttribute("aria-pressed",String(item===b));
  }
});
