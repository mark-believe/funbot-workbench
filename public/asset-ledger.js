"use strict";
(()=>{
 const snapshots=new Map();
 let generation=0,day="",windows=[],selected="day",daily=null,shown=null,stageRows=[],loading=false,lastCheck=0;
 const bjNow=()=>bjDate(Math.floor(Date.now()/1000));
 const all=(id,val)=>$(id).textContent=val;
 const groupName=g=>"第 "+g+" 组";
 const clock=b=>new Date(b.Time*1000).toLocaleTimeString("zh-CN",{timeZone:"Asia/Shanghai",hour12:false});
 const sum=a=>a.reduce((v,r)=>v+BigInt(r.Available),0n);
 function amount(v,signed=false){
  const n=BigInt(v),mag=n<0n?-n:n,sign=n<0n?"−":signed&&n>0n?"+":"";
  const exact=sign+tokenAmount(mag),whole=mag/10n**18n,frac=(mag%10n**18n).toString().padStart(18,"0");
  const rounded=(mag+5n*10n**13n)/(10n**14n);
  let display=(rounded/10000n).toLocaleString("en-US");
  const decimals=(rounded%10000n).toString().padStart(4,"0").replace(/0+$/,"");
  if(decimals)display+="."+decimals;
  if(whole===0n&&rounded===0n&&/[1-9]/.test(frac))display="<0.0001";
  return '<span title="'+exact+'" class="'+(signed?(n>0n?"asset-up":n<0n?"asset-down":""):"")+'">'+sign+esc(display)+'</span>';
 }
 function fetchSheet(b){
  if(!snapshots.has(b.Hash)){
   const p=api("/api/asset-balances?block="+b.Height+"&hash="+b.Hash).then(s=>{
    if(s.Block.Hash!==b.Hash||!s.Accounts?.length)throw Error("资产快照不完整");return s;
   }).catch(e=>{snapshots.delete(b.Hash);throw e});
   snapshots.set(b.Hash,p);
   if(snapshots.size>100)snapshots.delete(snapshots.keys().next().value);
  }
  return snapshots.get(b.Hash);
 }
 async function pair(a,b){
  const [start,end]=await Promise.all([fetchSheet(a),fetchSheet(b)]);
  const byAddr=new Map(start.Accounts.map(s=>[s.Address.toLowerCase(),s]));
  if(start.Accounts.length!==end.Accounts.length||byAddr.size!==end.Accounts.length)throw Error("两次快照账号数不符");
  const rows=end.Accounts.map((s,i)=>{const p=byAddr.get(s.Address.toLowerCase());if(!p||s.Group!==p.Group)throw Error("两次快照名册不符");return {s,p,i,d:BigInt(s.Available)-BigInt(p.Available)}});
  return {a,b,rows,before:sum(start.Accounts),after:sum(end.Accounts)};
 }
 function stageName(w){
  const gs=w.Groups?.map(groupName).join("、")||"";
  return (gs+(w.Unknown?(gs?"等（归属待核）":"归属待核"):""))+" · "+(w.Plays&&w.Collections?"打局及归集":w.Plays?"打局":"归集");
 }
 function renderAccounts(){
  if(!shown)return;
  const g=$("ledger-group").value,q=$("ledger-search").value.trim().toLowerCase();
  const rows=shown.rows.filter(({s})=>(!g||String(s.Group)===g)&&(!q||(s.Label+" "+s.Address).toLowerCase().includes(q)));
  all("ledger-detail-title","逐账号资产明细 · "+rows.length+" 个账号");
  $("ledger-accounts").innerHTML=rows.map(({s,p,i,d})=>'<tr><th title="'+esc(s.Address)+'">'+esc(s.Label||("账号 "+(i+1)))+' <small>'+esc(short(s.Address))+'</small></th>'+[amount(p.Available),amount(s.Available),amount(d,true),...["Wallet","Arena","Jiazi","Lineage","Fuze","Occupied"].map(k=>amount(s[k]))].map(v=>"<td>"+v+"</td>").join("")+"</tr>").join("")||'<tr><td colspan="10">没有匹配的账号</td></tr>';
 }
 function paint(pair){
  shown=pair;const {a,b,rows}=pair;
  all("ledger-time",clock(a)+" → "+clock(b)+" 北京时间 · 区块 "+a.Height+" → "+b.Height);
  const groups=[...new Set(rows.map(r=>r.s.Group))];
  const owners=windows[Number(selected)]?.Groups;
  const saved=owners?.length===1?String(owners[0]):"";
  $("ledger-group").innerHTML='<option value="">全部组</option>'+groups.map(g=>'<option value="'+g+'">'+groupName(g)+'</option>').join("");
  $("ledger-group").value=groups.includes(+saved)?saved:"";
  $("ledger-groups").innerHTML=groups.map(g=>{const items=rows.filter(r=>r.s.Group===g);
   const x=sum(items.map(r=>r.p)),y=sum(items.map(r=>r.s)),locked=items.reduce((n,r)=>n+BigInt(r.s.Occupied),0n);
   return '<tr><th><button class="group-link" data-ledger-group="'+g+'">'+groupName(g)+'</button></th>'+[amount(x),amount(y),amount(y-x,true),amount(locked)].map(s=>"<td>"+s+"</td>").join("")+'</tr>';
  }).join("");
  renderAccounts();
 }
 function renderStages(){
  $("ledger-stages").innerHTML=windows.map((w,i)=>{
   const val=stageRows[i];
   return "<tr><th>"+esc(stageName(w))+' <small>'+clock(w.After)+'</small></th>'+(val?.rows?[amount(val.before),amount(val.after),amount(val.after-val.before,true)].map(s=>"<td>"+s+"</td>").join(""):'<td colspan="3">'+(val?.error?'读取失败，点明细重试':'读取中…')+'</td>')+'<td><button type="button" class="group-link" data-stage="'+i+'">明细</button></td></tr>';
  }).join("");
 }
 function clearView(){
  shown=null;
  all("ledger-groups","");all("ledger-accounts","");all("ledger-detail-title","逐账号资产明细");
 }
 async function choose(value){
  const epoch=generation;selected=value;$("ledger-window").value=value;
  const full=value==="day",run=value==="run",w=full?daily:run?{Before:windows[0]?.Before,After:windows.at(-1)?.After}:windows[Number(value)];
  if(!w)return;
  if(!full&&!run)$("ledger-group").value=w.Groups?.length===1?String(w.Groups[0]):"";
  if(!shown||shown.a.Hash!==(full?w.Start:w.Before).Hash||shown.b.Hash!==(full?w.End:w.After).Hash){clearView();all("ledger-time","正在核对期初和期末可用资产…")}
  try{const valuePair=await pair(full?w.Start:w.Before,full?w.End:w.After);if(epoch===generation&&selected===value)paint(valuePair);}
  catch(e){if(epoch===generation&&selected===value){clearView();all("ledger-time","资产暂缺 · "+e.message);}}
 }
 function opts(){const sel=$("ledger-window");
  sel.innerHTML='<option value="day">北京时间全天</option>'+(windows.length?'<option value="run">当天首笔打前 → 末笔打后</option>':'')+windows.map((w,i)=>'<option value="'+i+'">'+esc(stageName(w))+" · "+clock(w.After)+'</option>').join("");sel.value=selected;
 }
 async function load(date,force=false){
  if(!date||date>bjNow())return;
  if(date===day&&!force&&(loading||Date.now()-lastCheck<55000))return;
  const changed=date!==day;const epoch=++generation;day=date;lastCheck=Date.now();loading=true;
  $("ledger-date").value=date;$("ledger-date").max=bjNow();
  snapshots.clear();
  for(const id of ["net","start","end"])all("ledger-"+id,"—");
  all("ledger-reconcile","");
  if(changed){selected="day";daily=null;windows=[];stageRows=[];opts();clearView();all("ledger-stages","");all("ledger-time","正在核实链上区块…")}
  all("ledger-stage-status","正在核对打局及归集记录…");
  try{
   const bounds=await api("/api/asset-windows?date="+date);
   if(epoch!==generation)return;daily=bounds;
   async function dailyTotals(bounds){
    const full=await pair(bounds.Start,bounds.End);
    if(epoch!==generation)return;
    all("ledger-end-label",bounds.Closed?"日末可用":"截至当前");
    for(const [id,v,delta]of [["net",full.after-full.before,true],["start",full.before,false],["end",full.after,false]])$("ledger-"+id).innerHTML=amount(v,delta);
    if(selected==="day")paint(full);
    return full.after-full.before;
   }
   let total=await dailyTotals(bounds);
   if(epoch!==generation)return;
   const timeline=await api("/api/asset-windows?mode=stages&date="+date);
   if(epoch!==generation)return;
   daily=timeline;
   if(timeline.End.Hash!==bounds.End.Hash||timeline.Start.Hash!==bounds.Start.Hash)total=await dailyTotals(timeline);
   if(epoch!==generation)return;
   windows=timeline.Stages||[];stageRows=[];opts();renderStages();
   if(selected!=="day"&&(!windows.length||selected!=="run"&&!windows[+selected]))selected="day";
   if(selected!=="day")await choose(selected);
   all("ledger-stage-status",windows.length?"按交易先后排列 · "+windows.length+" 个时段。点明细可查该组每个账号。":"截至区块 "+timeline.End.Height+"，主号当天没有打局或归集记录。");
   for(let i=0;i<windows.length&&epoch===generation;i++){
    const w=windows[i];let val;try{val=await pair(w.Before,w.After)}catch(e){val={error:e.message}}
    if(epoch!==generation)return;stageRows[i]=val;renderStages();
   }
   if(stageRows.every(v=>v?.rows)){
    const staged=stageRows.reduce((n,v)=>n+v.after-v.before,0n);
    $("ledger-reconcile").innerHTML="打局及归集时段合计 "+amount(staged,true)+"　·　时段外变动 "+amount(total-staged,true)+"　·　合计等于全队当日变动";
   }
  }catch(e){if(epoch===generation){if(!daily)all("ledger-time","日期读取失败 · "+e.message);all("ledger-stage-status","读取失败 · "+e.message)}}
  finally{if(epoch===generation)loading=false}
 }
 $("ledger-groups").onclick=e=>{const g=e.target.closest("[data-ledger-group]");if(g){$("ledger-group").value=g.dataset.ledgerGroup;$("ledger-detail").open=true;renderAccounts();$("ledger-detail").scrollIntoView({block:"nearest"})}};
 $("ledger-stages").onclick=e=>{const b=e.target.closest("[data-stage]");if(b){$("ledger-detail").open=true;choose(b.dataset.stage);$("ledger-detail").scrollIntoView({block:"center"})}};
 $("ledger-group").onchange=renderAccounts;$("ledger-search").oninput=renderAccounts;$("ledger-window").onchange=e=>choose(e.target.value);
 $("ledger-date").onchange=e=>{const date=e.target.value;if(date)load(date,true)};
 $("ds-refresh").addEventListener("click",()=>load(day||bjNow(),true));
 window.addEventListener("show-today-assets",()=>load(bjNow(),true));
 let lastToday=bjNow();setInterval(()=>{if(document.hidden||$("tab-stats").hidden||loading)return;const today=bjNow();if(day===today||day===lastToday)load(today);lastToday=today},15000);
 load(bjNow());
})();

