"use strict";
function assetUnits(value, signed = false) {
  const negative = value < 0n;
  const digits = (negative ? -value : value).toString().padStart(19, "0");
  const tail = digits.slice(-18).replace(/0+$/, "");
  return (negative ? "−" : signed && value > 0n ? "+" : "") + digits.slice(0,-18) + (tail ? "." + tail : "");
}
const assetForm = $("assetRange");
$("assetDay").value = bjDate(Date.now()/1000);
function resetAssetView() {
  $("assetRows").replaceChildren();
  $("assetStatus").textContent = "点击查询变动，读取所选区间。";
  const daily = $("assetMode").value === "day";
  $("assetDay").hidden = !daily;
  $("assetBounds").hidden = daily;
}
assetForm.oninput = resetAssetView;
assetForm.onsubmit = async event => {
  event.preventDefault();
  const inputs = [...assetForm.elements];
  inputs.forEach(x => x.disabled = true);
  $("assetRows").replaceChildren();
  $("assetStatus").textContent = "正在读取区间两端的链上资产…";
  try {
    const q = new URLSearchParams($("assetMode").value === "day" ? {date:$("assetDay").value} : {first:$("assetFirst").value,last:$("assetLast").value});
    const [before, after] = await api("/api/assets/change?" + q);
    const groups = new Map();
    let start = 0n, end = 0n;
    for (const [i,a] of before.Accounts.entries()) {
      const b = after.Accounts[i];
      if (a.Address !== b.Address) throw Error("前后名册不一致");
      const x = BigInt(a.Available), y = BigInt(b.Available);
      start += x; end += y;
      const group = groups.get(a.Group) || {x:0n,y:0n,rows:[]};
      group.x += x; group.y += y;
      group.rows.push("<tr><td>"+esc(a.Label)+" "+esc(a.Address)+"</td><td>"+assetUnits(x)+"</td><td>"+assetUnits(y)+"</td><td>"+assetUnits(y-x,true)+"</td><td>"+assetUnits(BigInt(b.Deposit))+"</td></tr>");
      groups.set(a.Group,group);
    }
    $("assetStatus").textContent = "全队净变化 "+assetUnits(end-start,true)+" fLGNS · 期初 "+assetUnits(start)+" → 期末 "+assetUnits(end)+" · 区块 "+before.Block+" → "+after.Block;
    for (const [g,v] of groups) {
      const details=document.createElement("details"), summary=document.createElement("summary");
      summary.textContent="第 "+g+" 组 · "+assetUnits(v.x)+" → "+assetUnits(v.y)+" · 净变化 "+assetUnits(v.y-v.x,true)+" · 账号明细";
      details.append(summary);
      details.insertAdjacentHTML("beforeend",'<div class="table-scroll"><table><thead><tr><th>账号</th><th>期初可用</th><th>期末可用</th><th>净变化</th><th>期末局内占用</th></tr></thead><tbody>'+v.rows.join("")+"</tbody></table></div>");
      $("assetRows").append(details);
    }
  } catch(e) { $("assetStatus").textContent = "未能完成对账："+e.message; }
  finally { inputs.forEach(x => x.disabled = false); }
};
