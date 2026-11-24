"use strict";

const vipDialog = $("vipDialog");
const vipForm = $("vipForm");
const vipButton = $("vipOpen");
const vipInput = $("vipCode");
const vipSubmit = $("vipSubmit");
const vipMessage = $("vipStatus");

function showVIPStatus(active) {
  vipButton.textContent = active ? "终身 VIP" : "社区 VIP";
  $("vipTitle").textContent = active ? "终身 VIP 已激活" : "MARK·AI 社区 VIP";
  vipForm.hidden = active;
  vipMessage.textContent = active ? "本机资格已生效，后续客户端升级继续使用。" : "";
}

vipButton.addEventListener("click", async () => {
  vipDialog.showModal();
  if (demoMode) {
    vipInput.disabled = true;
    vipSubmit.disabled = true;
    vipMessage.textContent = "当前为测试数据模式，请切换到真实环境提交授权码。";
    return;
  }
  try {
    const result = await api("/api/vip");
    if (vipDialog.open && !vipSubmit.disabled) showVIPStatus(result.active);
  } catch (error) {
    if (vipDialog.open && !vipSubmit.disabled) vipMessage.textContent = error.message;
  }
});

vipForm.addEventListener("submit", async event => {
  event.preventDefault();
  if (demoMode || vipSubmit.disabled) return;
  vipSubmit.disabled = true;
  vipMessage.textContent = "正在验证授权码…";
  const code = vipInput.value.trim();
  vipInput.value = "";
  try {
    const result = await api("/api/vip/activate", { code });
    showVIPStatus(result.active);
  } catch (error) {
    vipMessage.textContent = error.message;
  } finally {
    vipSubmit.disabled = false;
  }
});

vipDialog.addEventListener("close", () => { vipInput.value = ""; });
if (!demoMode && TOKEN) api("/api/vip").then(result => {
  if (!vipDialog.open) showVIPStatus(result.active);
}).catch(() => {});
