"use strict";
function fuzeFailure(status) {
  const messages={tls:"未能和节点建立安全连接",network:"节点连接中断或超时",rpc:"节点未能完成链上查询",audit:"链上数据尚未核验通过"};
  const message=messages[status.code] || (/exit status 35\b/.test(status.error||"")?messages.tls:"检查未完成");
  const time=Date.parse(status.at);
  return "上次检查失败"+(Number.isFinite(time)?"（"+chainTime(time/1000)+" 北京时间）":"")+" · "+message;
}
function setDailySnapshot(note) {
  put("df-snapshot",note);
  $("df-snapshot").hidden=!note;
}
function clearDailyValues() {
  for(const id of ["df-amount","df-gas","df-actual","df-paid"])put(id,"—");
  put("df-scope","");setHTML("df-ledger","");
  setDailySnapshot("");
}
function showDailyReport(data) {
  const r=data.latest,failed=data.status?.ok===false;
  if(!data.available||!r){
    clearDailyValues();
    put("df-status",!data.available?"此模拟任务仅绑定原主号":failed?fuzeFailure(data.status):"等待首次检查");
    return;
  }
  const summary="第 "+r.period+" 期 · 持有 "+r.held+"/42 枚 · 可领 "+r.eligible.length+" 枚";
  const time=chainTime(r.timestamp)+"（北京时间）";
  const dateCN=t=>new Date(t+8*3600000).toISOString().slice(0,10);
  const fresh=!failed && dateCN(Date.now())===dateCN(r.timestamp*1000);
  put("df-status",fresh?summary+" · "+time:failed?fuzeFailure(data.status):"今日尚无成功检查 · 每日 09:00（北京时间）检查");
  setDailySnapshot(fresh?"":"上次成功核验于 "+time+"，"+summary+"。累计到账及 Gas 截至该时点。");
  put("df-amount",fresh?tokenAmount(r.amount)+" fLGNS":"待核验");
  put("df-gas",fresh?r.gas.toLocaleString("en-US")+" Gas":"待核验");
  put("df-actual",tokenAmount(r.actualAmount)+" fLGNS");
  put("df-paid",r.actualGas.toLocaleString("en-US")+" Gas");
  put("df-scope","每日 09:00（北京时间）检查。"+(fresh?"本次":"上次成功")+"核验区块 "+r.block.toLocaleString("en-US")+"。当时可领编号："+(r.eligible.map(n=>"#"+n).join("、")||"无")+"。模拟手续费 "+tokenAmount(r.fee)+" DAI。实际金额与费用统计自区块 配置的起始区块 起，由主号直接领取清单内令牌的成功交易，累计实付 "+tokenAmount(r.actualFee)+" DAI。"+(r.missing.length?"已移出主号："+r.missing.join("、"):""));
  setHTML("df-ledger",r.ledger.length?r.ledger.slice().reverse().map(tx=>'<p>第 '+esc(tx.period)+' 期 · '+tokenAmount(tx.amount)+' fLGNS · '+Number(tx.gas).toLocaleString("en-US")+' Gas · '+tokenAmount(tx.fee)+' DAI<br><code>'+esc(tx.hash)+'</code></p>').join(""):"尚无符合范围的领取回执");
}
let dailyLoading=false;
async function loadDailyReport(){
  if(dailyLoading||document.hidden||tab!=="stats")return;
  dailyLoading=true;
  try{showDailyReport(await api("/api/daily-fuze"));}
  catch(e){clearDailyValues();put("df-status","暂时无法读取本地福泽报告，请点「刷新统计」重读。");}
  finally{dailyLoading=false;}
}
$("assetDaily").onclick=()=>{showTab("stats");showReport("rewards");$("daily-fuze").scrollIntoView({block:"center"});};
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
