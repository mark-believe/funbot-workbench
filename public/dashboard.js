"use strict";
const rewardLabels = ["本期未纳入","待启","可领取","已领取","已过期","未开奖","暂缓"];
const rewardColors = ["#74869a","#dda54b","#167966","#4d78b9","#bc675b","#977ab3","#929891"];
let report, lastRead=0, loadingStats=false, expanded="";
const tokens = a => a.Tokens || [];
const mintCount = a => (a.Mint || []).length;
const claimable = a => tokens(a).filter(t=>t.Status===2);
function metric(a,key){
 if(key==="wins") return a.Days?.[selectedDay()]?.Wins || 0;
 if(key==="nft") return tokens(a).length;
 if(key==="mint") return mintCount(a);
 if(key==="claim") return claimable(a).length;
 return a.Games;
}
function tokenAmount(wei){
 const n=BigInt(wei);
 const fraction=(n%1000000000000000000n).toString().padStart(18,"0").replace(/0+$/,"");
 return (n/1000000000000000000n).toLocaleString("en-US")+(fraction?"."+fraction:"");
}
function chainTime(ts){return new Date(ts*1000).toLocaleString("zh-CN",{timeZone:"Asia/Shanghai",hour12:false});}
async function refreshDashboard(force=false){
 if(loadingStats||(!force && Date.now()-lastRead<45000))return;
 loadingStats=true;$("ds-refresh").disabled=true;
 put("ds-status",report?"刷新中 · 上次快照 "+chainTime(report.Timestamp):"正在逐组读取链上统计…");
 try{
  const next=await api("/api/dashboard");
  if(!Array.isArray(next.Accounts)) throw Error("统计响应不完整");
  report=next;lastRead=Date.now();
  renderDashboard();
  put("ds-status","链上区块 "+report.Block.toLocaleString("en-US")+" · "+chainTime(report.Timestamp)+" 北京时间 · 每分钟更新");
 }catch(e){put("ds-status","刷新失败 · "+(report?"保留 "+chainTime(report.Timestamp)+" 的快照 · ":"")+e.message);}
 finally{loadingStats=false;$("ds-refresh").disabled=false;}
}
function renderDashboard(){
 const seats=report.Accounts, groups=[...new Set(seats.map(a=>a.Group))];
 renderMatchSummary(seats,groups);
 renderExpansion();
 const old=$("ds-group").value;
 setHTML("ds-group",'<option value="">全部组</option>'+groups.map(g=>'<option value="'+g+'">第 '+g+' 组</option>').join(""));$("ds-group").value=groups.includes(Number(old))?old:"";
 const dist=rewardLabels.map(()=>0);let sum=0n;
 for(const a of seats)for(const t of tokens(a)){dist[t.Status]++;if(t.Status===2)sum+=BigInt(t.Amount);}
 const total=dist.reduce((a,b)=>a+b,0);let offset=0;
 const stops=dist.flatMap((n,i)=>{if(!n)return [];const from=offset;offset+=100*n/total;return [rewardColors[i]+" "+from+"% "+offset+"%"];});
 setHTML("ds-distribution",'<div class="ring-wrap"><div class="ring" style="background:'+(stops.length?'conic-gradient('+stops.join(",")+')':'#e7ece9')+'"><strong>'+total+'<small>枚 NFT</small></strong></div><div class="legend">'+dist.map((n,i)=>n?'<div><i style="background:'+rewardColors[i]+'"></i>'+rewardLabels[i]+' <b>'+n+'</b></div>':'').join("")+(total?'':'暂无持仓')+'</div></div>');
 put("ds-period",report.Period?'第 '+report.Period+' 期 · 可领 '+tokenAmount(sum)+' fLGNS · 截止 '+chainTime(report.Deadline)+'（北京）':'暂无期次');
 renderAccounts();
}
function renderAccounts(){
 if(!report)return;
 const group=$("ds-group").value,f=$("ds-filter").value,q=$("ds-search").value.trim().toLowerCase();
 const rows=report.Accounts.filter(a=>(!group||String(a.Group)===group)&&(!f||metric(a,f)>0)&&(!q||[a.Label,a.Address,a.Kind==="master"?"主号":"",...tokens(a).map(t=>"#"+t.ID)].join(" ").toLowerCase().includes(q)));
 put("ds-count",rows.length+" / "+report.Accounts.length+" 个账号");
 const html=rows.map(a=>{
 const open=expanded===a.Address;const claims=claimable(a);
 let row='<tr><td><button class="account-link" aria-expanded="'+open+'" data-account="'+esc(a.Address)+'">'+esc(a.Label||(a.Kind==="master"?"主号":"子号"))+'</button><small>'+esc(short(a.Address))+'</small></td><td>'+a.Group+'</td><td>'+winsThrough(a,selectedDay())+'</td><td class="wins-cell">'+(a.Days?.[selectedDay()]?.Wins||0)+'</td><td>'+(a.Days?.[selectedDay()]?.Games||0)+'</td><td>'+mintCount(a)+'</td><td>'+tokens(a).length+'</td><td>'+claims.length+' 枚</td></tr>';
 if(open)row+='<tr class="detail"><td colspan="8">'+addressHTML(a.Address)+'<p>钱包 '+tokenAmount(a.Wallet)+' fLGNS · 可铸甲子俸索引 '+((a.Mint||[]).join("、")||"无")+'</p><p>本期可领 '+tokenAmount(claims.reduce((n,t)=>n+BigInt(t.Amount),0n))+' fLGNS</p><div class="token-list">'+(tokens(a).map(t=>'<span>#'+esc(t.ID)+' · '+rewardLabels[t.Status]+'</span>').join("")||"暂无 NFT")+'</div></td></tr>';
 return row;}).join("");
 setHTML("ds-rows",html||'<tr><td colspan="8">当前筛选没有匹配的账号</td></tr>');
}
function resetPage(){expanded="";renderAccounts();document.querySelector(".account-report .table-scroll").scrollTop=0;}
$("ds-refresh").onclick=()=>refreshDashboard(true);
$("ds-search").oninput=resetPage;
$("ds-filter").onchange=$("ds-group").onchange=resetPage;
$("tab-stats").addEventListener("click",e=>{
 const btn=e.target.closest("button");if(!btn)return;
 if(btn.dataset.account){expanded=expanded===btn.dataset.account?"":btn.dataset.account;renderAccounts();}
 if("group" in btn.dataset||"filter" in btn.dataset){
  if("group" in btn.dataset){$("ds-group").value=btn.dataset.group;$("ds-filter").value="";$("ds-search").value="";}
  if("filter" in btn.dataset){$("ds-filter").value=btn.dataset.filter;$("ds-group").value="";$("ds-search").value="";}
  resetPage();$("ds-search").scrollIntoView({block:"center"});
 }
});
document.querySelectorAll("[data-go]").forEach(b=>b.onclick=()=>showTab(b.dataset.go));
setInterval(()=>{if(!document.hidden && tab==="stats")refreshDashboard();},60000);
document.addEventListener("visibilitychange",()=>{if(!document.hidden&&tab==="stats")refreshDashboard();});
