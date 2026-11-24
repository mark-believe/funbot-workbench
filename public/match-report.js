"use strict";
const DAY_MS=86400000;
let historicalDay="";
const countFormat=n=>n.toLocaleString("en-US");
const bjDate=ts=>new Date(ts*1000+8*3600000).toISOString().slice(0,10);
function selectedDay(){return historicalDay || bjDate(report.Timestamp);}
function winsThrough(a,date){
 if(date===bjDate(report.Timestamp))return a.Wins;
 return Object.entries(a.Days||{}).reduce((sum,[day,d])=>sum+(day<=date?d.Wins:0),0);
}
function matchTotals(accounts,date){
 const t={wins:0,newWins:0,games:0,settled:0};
 for(const a of accounts){
  t.wins+=winsThrough(a,date);
  const d=a.Days?.[date];if(!d)continue;
  t.newWins+=d.Wins;t.games+=d.Games;t.settled+=d.Settled;
 }
 return t;
}
function renderMatchSummary(seats,groups){
 const today=bjDate(report.Timestamp),date=selectedDay();
 const picker=$("match-date");picker.max=today;picker.value=date;
 const summary=matchTotals(seats,date);
 const cards=[["全队累计盟主",summary.wins,"次",""],["当日新增盟主",summary.newWins,"次","wins"],
  ["当日参赛",summary.games,"人次",""],["当前可铸令",seats.reduce((s,a)=>s+mintCount(a),0),"枚","mint"],
  ["当前持有 NFT",seats.reduce((s,a)=>s+tokens(a).length,0),"枚","nft"],["本期可领福泽",seats.reduce((s,a)=>s+claimable(a).length,0),"枚","claim"]];
 setHTML("ds-cards",cards.map(([label,n,unit,filter])=>'<button type="button" class="metric" data-filter="'+filter+'"><span>'+label+'</span><strong>'+countFormat(n)+'<small>'+unit+'</small></strong></button>').join(""));
 const cells=(name,t)=>'<tr><th scope="row">'+name+'</th><td>'+countFormat(t.wins)+'</td><td class="wins-cell">+'+countFormat(t.newWins)+'</td><td>'+countFormat(t.games)+'</td><td>'+countFormat(t.games-t.settled)+'</td><td>'+(t.settled?(100*t.newWins/t.settled).toFixed(1)+"%":"—")+'</td></tr>';
 setHTML("match-groups",groups.map(g=>cells('<button class="group-link" data-group="'+g+'">第 '+g+' 组</button>',matchTotals(seats.filter(a=>a.Group===g),date))).join(""));
 setHTML("match-total",cells("全队合计",summary));
 const week=Array.from({length:7},(_,i)=>{
  const day=new Date(Date.parse(date+"T00:00:00Z")+(i-6)*DAY_MS).toISOString().slice(0,10);
  const byGroup=groups.map(g=>seats.reduce((sum,a)=>sum+(a.Group===g?(a.Days?.[day]?.Wins||0):0),0));
  return {day,byGroup,count:byGroup.reduce((sum,n)=>sum+n,0)};
 }).reverse();
 const peak=Math.max(1,...week.flatMap(r=>r.byGroup));
 const header='<tr><th>日期</th>'+groups.map(g=>'<th>'+g+' 组</th>').join("")+'<th>全队</th></tr>';
 const body=week.map(r=>'<tr class="'+(date===r.day?'active-day':'')+'"><th><button type="button" class="group-link" data-day="'+r.day+'" aria-pressed="'+(date===r.day)+'">'+r.day.slice(5)+'</button></th>'+r.byGroup.map(n=>'<td style="background-color:rgba(48,141,110,'+(.30*n/peak)+')">'+n+'</td>').join("")+'<td><b>'+r.count+'</b></td></tr>').join("");
 setHTML("match-week",'<table class="week-table"><thead>'+header+'</thead><tbody>'+body+'</tbody></table>');
}
function pickMatchDay(day){
 if(!report)return;
 const today=bjDate(report.Timestamp);
 if(day>today){$("match-date").value=selectedDay();return;}
 historicalDay=day===today?"":day;
 renderDashboard();
}
$("match-date").onchange=e=>pickMatchDay(e.target.value);
$("match-week").onclick=e=>{const button=e.target.closest("button[data-day]");if(button)pickMatchDay(button.dataset.day);};
