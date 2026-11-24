"use strict";

const demoMode = location.hash === "#demo";
const initialWorkbenchTab = history.state?.workbenchTab;
const environmentSelect = document.getElementById("environment");
environmentSelect.value = demoMode ? "demo" : "live";
document.body.classList.toggle("demo-mode", demoMode);
environmentSelect.onchange = () => {
  const next = new URL(location.href);
  next.hash = environmentSelect.value === "demo" ? "demo" : "";
  const active = document.querySelector(".tab.on")?.dataset.tab;
  history.replaceState({workbenchTab:active}, "", next);
  location.reload();
};

// A fresh document owns all filters, confirmations, account selections and caches.
const demoAPI = demoMode ? createDemoAPI() : null;

function createDemoAPI() {
  const now = Math.floor(Date.now()/1000);
  const dateOf = time => new Date((time+28800)*1000).toISOString().slice(0,10);
  const today = dateOf(now);
  const address = n => "0x"+"de".repeat(16)+n.toString(16).padStart(8,"0");
  const wei = n => String(BigInt(n)*10n**18n);
  const groups = [1,2,3];
  const seats = Array.from({length:72}, (_,i) => ({
    index:i, address:address(i+1), kind:i?"member":"master",
    label:i?"测试 "+(1+Math.floor(i/24))+" 组 · "+String(i%24+1).padStart(2,"0"):"测试主号"
  }));
  const master=seats[0].address, contract=address(1001);
  function sheet(date,step){
    const midnight=Date.parse(date+"T00:00:00+08:00")/1000;
    const span=Math.min(86399,Math.max(0,now-midnight));
    const time=step===0?midnight-1:midnight+Math.floor(step*span/4);
    const block={Time:time,Height:Math.floor(midnight/86400)*10+step,Hash:date+"-"+step};
    return {Block:block,Accounts:seats.map((s,i)=>{
      const group=1+Math.floor(i/24);
      const delta=step>=group?[6,-4,0][group-1]:0;
      const balance=50+i+delta+(i===0&&step===4?3:0);
      return {Address:s.address,Label:s.label,Group:group,Wallet:wei(balance),Arena:wei(3),Lineage:wei(1),
        Occupied:"0",Deposit:"0",Jiazi:wei(2),Fuze:"0",Available:wei(balance+6)};
    })};
  }
  const current=sheet(today,4);
  const accounts=current.Accounts.map((a,i)=>{
    const days={};
    for(let d=0;d<7;d++)days[dateOf(now-d*86400)]={Games:12,Settled:d?12:10,Wins:(i+d)%3};
    return {...a,Kind:seats[i].kind,Days:days,Games:84,Wins:Object.values(days).reduce((n,v)=>n+v.Wins,0),
      Stipends:14-3*a.Group,ActiveStipends:13-3*a.Group,Mint:i%9===0?[0]:[],
      Tokens:i%8===0?[{ID:String(7000+i),Status:i%3+1,Amount:wei(4)}]:[]};
  });
  const state={
    systemName:"布局测试",net:"模拟环境",block:current.Block.Height,master,contract,masterReady:true,hasVault:true,locked:false,
    seats,settings:{all:true,masterPlays:true,entryMode:"flexible",rounds:12},balancesUpdated:"模拟快照",
    config:{master,impl:contract,arena:address(1002),token:address(1003),shifu:address(1004)},
    overview:{masterBalance:"59",masterGas:"0.42",roundId:812},
    log:[{at:"09:00:00",kind:"ok",text:"已加载 3 组模拟账户，可切换各模块预览。"}],
    delegations:Object.fromEntries(seats.map((s,i)=>[s.address,i%17!==16])),
    mnemonicSources:[{id:master,label:"测试钱包组",nextIndex:72,wallets:seats.map(s=>({...s,path:"m/44'/60'/0'/0/"+s.index}))}],
    rows:seats.map((s,i)=>({who:s.address,delegated:i%17!==16,championships:accounts[i].Wins,seated:i%7===1,
      seatNo:i%12+1,openSeats:5,seats:12,matchesToday:i%13,blocksToWait:i%5===3?2:0,wallet:String(BigInt(accounts[i].Wallet)/10n**18n),
      settled:"3",unsettled:"0",lineage:"1",jiazi:"2",ready:"6",gasBalance:"0.08",canHarvest:true}))
  };
  const manifest=Object.fromEntries(groups.map(g=>[g,accounts.filter(a=>a.Group===g).map(a=>a.Address)]));
  const book=Object.fromEntries(groups.map(g=>[g,{state:["可开一组","未回本","待核算"][g-1],gap:g===2?140:0,capital:1000,
    profit:g===1?1250:860,spare:1200/g,fundingGap:0,date:today}]));
  return async function(path,body){
    const url=new URL(path,location.origin),q=url.searchParams,date=q.get("date")||today;
    if(body!==undefined){
      if(url.pathname==="/api/settings"){state.settings={...state.settings,...body};return state}
      throw Error("测试数据用于预览，此操作仅在真实环境中执行。");
    }
    switch(url.pathname){
      case "/api/state": return state;
      case "/api/dashboard": return {Accounts:accounts,Block:state.block,Timestamp:now,Period:3,Deadline:now+86400};
      case "/api/mentors": return Object.fromEntries(seats.map(s=>[s.address,master]));
      case "/api/expansion": return {manifest:{groups:manifest},book:{groups:book}};
      case "/api/daily-fuze": return {available:true,latest:{timestamp:now,block:state.block,period:3,held:3,eligible:[7000],missing:[],
        amount:wei(4),gas:68000,fee:"816000000000000",actualAmount:wei(12),actualGas:136000,actualFee:"1632000000000000",ledger:[]}};
      case "/api/mandate/collection-plan":{
        const items=accounts.slice(1).flatMap(a=>a.Tokens.map(t=>({Source:a.Address,ID:t.ID})));
        return {plan:{Target:master,Items:items},approval:"demo",maxFeeWei:String(BigInt(items.length)*1440000000000000n)};
      }
      case "/api/asset-windows":{
        const blocks=Array.from({length:5},(_,i)=>sheet(date,i).Block);
        return {Start:blocks[0],End:blocks[4],Closed:date!==today,
          Stages:groups.map(g=>({Groups:[g],Before:blocks[g-1],After:blocks[g],Plays:12,Collections:1}))};
      }
      case "/api/asset-balances":{
        const hash=q.get("hash");return sheet(hash.slice(0,10),Number(hash.slice(11)));
      }
      case "/api/assets/change":{
        const samples=[sheet(date,0),sheet(date,4)];
        if(q.has("first")){
          const bounds=[Number(q.get("first"))-1,Number(q.get("last"))];
          if(!bounds.every(Number.isSafeInteger)||bounds[0]<0||bounds[0]>=bounds[1])throw Error("请输入有效的首末区块。");
          return bounds.map(h=>{
            const day=dateOf(Math.floor(h/10)*86400);
            if(h%10>4)throw Error("请选择模拟时段内的区块。");
            return {...sheet(day,h%10),Block:h};
          });
        }
        return samples.map(s=>({...s,Block:s.Block.Height}));
      }
      default:throw Error("此模块暂无测试数据");
    }
  };
}
