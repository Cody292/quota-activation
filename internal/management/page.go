package management

import (
	"net/http"

	"quota-activation/internal/state"
)

func (h *Handler) handlePage(w http.ResponseWriter) {
	response := h.latestResponse()
	status := latestStatus(response.Status)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>配额唤醒</title>
  <style>
    :root { color-scheme: light; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
    body { margin: 0; background: #f8fafc; color: #111827; }
    main { max-width: 760px; margin: 0 auto; padding: 48px 24px; }
    section { background: #fff; border: 1px solid #e5e7eb; border-radius: 16px; padding: 24px; box-shadow: 0 12px 30px rgba(15, 23, 42, .08); }
    h1 { margin: 0 0 12px; font-size: 28px; }
    p { line-height: 1.7; color: #4b5563; }
    code { background: #f3f4f6; border-radius: 6px; padding: 2px 6px; }
    label { display: block; margin: 18px 0 8px; color: #374151; font-weight: 600; }
    input, select { width: 100%; box-sizing: border-box; border: 1px solid #d1d5db; border-radius: 10px; padding: 12px; font: inherit; }
    .actions { display: flex; flex-wrap: wrap; gap: 12px; margin-top: 16px; }
    .message { min-height: 24px; margin-top: 14px; color: #374151; white-space: pre-wrap; }
    button { min-height: 44px; border: 0; border-radius: 10px; padding: 0 18px; background: #111827; color: #fff; font-weight: 600; cursor: pointer; }
    button.secondary { background: #e5e7eb; color: #111827; }
  </style>
</head>
<body>
  <main>
    <section>
      <h1>配额唤醒</h1>
      <p>当前状态：<strong>` + state.Redact(status) + `</strong></p>
      <p>插件支持自动唤醒与手动唤醒；默认手动，因为 <code>auto_activate=false</code>。</p>
      <div id="loginGate">
        <label for="managementKey">CPA 管理密钥</label>
        <input id="managementKey" type="password" autocomplete="current-password" placeholder="输入 CPA 管理密钥">
        <div class="actions"><button type="button" class="secondary" onclick="verifyManagementKey()">验证密钥</button></div>
      </div>
      <div id="appShell" hidden>
        <label for="credentialSelect">凭证</label><select id="credentialSelect" onchange="syncModelOptions()"></select>
        <label for="modelSelect">唤醒模型</label><select id="modelSelect"></select>
        <div class="actions"><button type="button" class="secondary" onclick="loadAuthFiles()">刷新凭证</button><button type="button" onclick="triggerActivation()">手动触发唤醒</button></div>
      </div>
      <p id="managementMessage" class="message" role="status"></p>
      <p>手动触发接口：<code>POST /v0/management/quota-activation/activate</code>。页面会根据所选凭证和模型生成完整请求。</p>
    </section>
  </main>
  <script>
    const STATUS_PATH = "/v0/management/quota-activation/status";
    const AUTH_FILES_PATH = "/v0/management/quota-activation/auth-files";
    const ACTIVATE_PATH = "/v0/management/quota-activation/activate";
    let authFiles = [];
    function message(text) { document.getElementById("managementMessage").textContent = text; }
    function managementKey() { const key = document.getElementById("managementKey").value.trim(); if (!key) { message("请先输入 CPA 管理密钥。"); } return key; }
    async function managementFetch(path, options) { const key = managementKey(); if (!key) { return null; } const response = await fetch(path, { ...(options || {}), headers: { "Authorization": "Bearer " + key, "Content-Type": "application/json", ...((options && options.headers) || {}) } }); const text = await response.text(); if (!response.ok) { throw new Error(text || response.statusText); } return text ? JSON.parse(text) : {}; }
    async function verifyManagementKey() { try { await managementFetch(STATUS_PATH, { method: "GET" }); document.getElementById("loginGate").hidden = true; document.getElementById("appShell").hidden = false; await loadAuthFiles(); message("管理密钥验证通过。"); } catch (error) { message("管理密钥验证失败：" + error.message); } }
    async function loadAuthFiles() { const result = await managementFetch(AUTH_FILES_PATH, { method: "GET" }); if (!result) { return; } authFiles = Array.isArray(result.files) ? result.files : []; const select = document.getElementById("credentialSelect"); select.innerHTML = ""; for (const file of authFiles) { const option = document.createElement("option"); option.value = file.auth_id; option.textContent = [file.provider, file.auth_id, file.disabled ? "已禁用" : "可用"].filter(Boolean).join(" · "); select.appendChild(option); } syncModelOptions(); if (authFiles.length === 0) { message("未发现可唤醒凭证。"); } }
    function syncModelOptions() { const credential = selectedCredential(); const modelSelect = document.getElementById("modelSelect"); modelSelect.innerHTML = ""; const models = credential && Array.isArray(credential.models) ? credential.models : []; for (const item of models) { const option = document.createElement("option"); option.value = item.value; option.dataset.group = item.group || ""; option.textContent = item.label || item.value; modelSelect.appendChild(option); } if (credential && models.length === 0) { message("该凭证没有启用的唤醒模型组。"); } }
    function selectedCredential() { const authId = document.getElementById("credentialSelect").value; return authFiles.find((item) => item.auth_id === authId) || null; }
    function buildManualActivationRequest() { const credential = selectedCredential(); if (!credential) { throw new Error("请先选择凭证。"); } const modelOption = document.getElementById("modelSelect").selectedOptions[0]; if (!modelOption) { throw new Error("请先选择已启用的唤醒模型。"); } return { auth_id: credential.auth_id, provider: credential.provider, model_group: modelOption.dataset.group || "", model: modelOption.value, disabled: Boolean(credential.disabled), quota_payload: credential.quota_payload }; }
    async function triggerActivation() { try { const result = await managementFetch(ACTIVATE_PATH, { method: "POST", body: JSON.stringify(buildManualActivationRequest()) }); if (!result) { return; } message("手动唤醒请求已完成：" + JSON.stringify(result, null, 2)); } catch (error) { message("手动唤醒失败：" + error.message); } }
  </script>
</body>
</html>`))
}
