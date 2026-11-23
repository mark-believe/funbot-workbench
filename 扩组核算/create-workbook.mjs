import fs from "node:fs/promises";
import {Workbook,SpreadsheetFile} from "@oai/artifact-tool";
const wb=Workbook.create(), sheet=wb.worksheets.add("分组核算");
const notes=[
 "分组打正与扩组核算  |  单位 fLGNS",
 "每行是一个组截至当日的累计快照。首次填写请补日期，后续日期追加新行；同组同日修改原行。蓝字为输入，未知金额留空。",
 "累计净收益填已实现收入扣除打局净损耗、运营费与折算 Gas 后的累计净额，可为负；返还本金、组内划转、待领与未到期甲子不算收益。",
 "可调配余额仅填本组已归属且可支配的现金。主号共享余额只分配一次；老组预留含周转金及需保留的本金。费用折算依据写入备注。",
 "打正：净收益 ≥ 投入；可开一组还需可调配余额 − 老组预留 ≥ 新组预算。250 条甲子是独立经验线。总览读取已保存的数据。"
];
for(let i=0;i<notes.length;i++){sheet.getRange("A"+(i+1)+":H"+(i+1)).merge();sheet.getRange("A"+(i+1)).values=[[notes[i]]];}
sheet.getRange("A1:H1").format.fill="#174F46";sheet.getRange("A1:H1").format.font={bold:true,size:17,color:"#FFFFFF"};sheet.getRange("A1:H1").format.rowHeight=34;
sheet.getRange("A2:H5").format.font={size:10,color:"#34554D"};sheet.getRange("A2:H5").format.rowHeight=23;
sheet.getRange("A7:H7").values=[["核算日期","组号","累计投入","累计已实现净收益","可调配余额","老组预留","开一组预算","口径 / 凭证备注"]];
sheet.getRange("A7:H7").format.fill="#256C60";sheet.getRange("A7:H7").format.font={color:"#FFFFFF",bold:true};
sheet.getRange("B8:B13").values=Array.from({length:6},(_,i)=>[i+1]);
sheet.getRange("A8:H140").format.font={color:"#245B9D"};sheet.getRange("A8:A140").setNumberFormat("yyyy-mm-dd");
sheet.getRange("C8:G140").setNumberFormat("#,##0.########;[Red](#,##0.########);0");
["A","B","C","D","E","F","G","H"].forEach((c,i)=>sheet.getRange(c+"7").format.columnWidth=[15,9,17,22,18,16,17,32][i]);
sheet.showGridLines=true;sheet.freezePanes.freezeRows(7);
const dir=new URL("./",import.meta.url);await fs.mkdir(dir,{recursive:true});
const file=new URL("分组核算.xlsx",dir);try{await fs.access(file);throw Error("核算表已存在，停止覆盖")}catch(e){if(e.code!=="ENOENT")throw e}
const preview=await wb.render({sheetName:"分组核算",range:"A1:H13",scale:0.8,format:"png"});
await fs.writeFile(new URL("expansion-preview.png",import.meta.url),new Uint8Array(await preview.arrayBuffer()));
await (await SpreadsheetFile.exportXlsx(wb)).save(decodeURIComponent(file.pathname).replace(/^\/([A-Z]:)/i,"$1"));
