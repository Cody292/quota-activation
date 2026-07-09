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
	    .topbar { display: flex; justify-content: flex-end; margin-bottom: 18px; }
	    .actions { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 12px; margin-top: 16px; }
	    .message { min-height: 24px; margin-top: 14px; color: #374151; white-space: pre-wrap; }
	    button { min-height: 44px; border: 0; border-radius: 10px; padding: 0 18px; background: #111827; color: #fff; font-weight: 600; cursor: pointer; }
	    button.secondary { background: #e5e7eb; color: #111827; }
	    button.link { min-height: 36px; background: transparent; color: #374151; border: 1px solid #d1d5db; }
	  </style>
</head>
<body>
	  <main>
	    <section>
	      <div id="loginGate">
	        <input id="managementKey" type="password" autocomplete="current-password" data-i18n-placeholder="managementKeyPlaceholder" placeholder="输入 CPA 管理密钥">
	        <div class="actions"><button type="button" data-i18n="verifyKey" onclick="verifyManagementKey()">验证密钥</button></div>
	      </div>
	      <div id="appShell" hidden>
	        <div class="topbar"><button id="languageSwitch" type="button" class="link" onclick="switchLanguage()">English</button></div>
	        <h1 data-i18n="title">配额唤醒</h1>
	        <p><span data-i18n="currentStatus">当前状态</span>：<strong id="currentStatusValue" data-status="` + state.Redact(status) + `"></strong></p>
	        <label for="credentialSelect" data-i18n="credential">凭证</label><select id="credentialSelect" onchange="syncModelOptions()"></select>
	        <label for="modelSelect" data-i18n="model">唤醒模型</label><select id="modelSelect"></select>
	        <div class="actions"><button type="button" class="secondary" data-i18n="refresh" onclick="loadAuthFiles()">刷新凭证</button><button type="button" data-i18n="activate" onclick="triggerActivation()">手动触发唤醒</button></div>
	      </div>
	      <p id="managementMessage" class="message" role="status"></p>
	    </section>
	  </main>
	  <script>
	    const STATUS_PATH = "/v0/management/quota-activation/status";
	    const AUTH_FILES_PATH = "/v0/management/quota-activation/auth-files";
	    const ACTIVATE_PATH = "/v0/management/quota-activation/activate";
	    const translations = {
	      "zh-CN": {
	        activate: "手动触发唤醒",
	        activationDone: "手动唤醒请求已完成。",
	        activationFailed: "手动唤醒失败，请稍后重试或检查凭证状态。",
	        credential: "凭证",
	        currentStatus: "当前状态",
	        disabled: "已禁用",
	        keyFailed: "管理密钥验证失败，请检查后重试。",
	        keyPassed: "管理密钥验证通过。",
	        managementKeyPlaceholder: "输入 CPA 管理密钥",
	        missingCredential: "请先选择凭证。",
	        missingKey: "请先输入 CPA 管理密钥。",
	        missingModel: "请先选择已启用的唤醒模型。",
	        model: "唤醒模型",
	        noAuthFiles: "未发现可唤醒凭证。",
	        noModels: "该凭证没有启用的唤醒模型组。",
	        refresh: "刷新凭证",
	        switchLanguage: "English",
	        statusBusy: "执行中",
	        statusFailed: "失败",
	        statusIdle: "空闲",
	        statusSkipped: "已跳过",
	        statusSuccess: "成功",
	        statusUnknown: "未知",
	        title: "配额唤醒",
	        usable: "可用",
	        verifyKey: "验证密钥"
	      },
	      "en-US": {
	        activate: "Trigger activation",
	        activationDone: "Manual activation request completed.",
	        activationFailed: "Manual activation failed. Try again later or check the credential status.",
	        credential: "Credential",
	        currentStatus: "Current status",
	        disabled: "Disabled",
	        keyFailed: "Management key verification failed. Check it and try again.",
	        keyPassed: "Management key verified.",
	        managementKeyPlaceholder: "Enter CPA management key",
	        missingCredential: "Select a credential first.",
	        missingKey: "Enter the CPA management key first.",
	        missingModel: "Select an enabled activation model first.",
	        model: "Activation model",
	        noAuthFiles: "No activatable credentials found.",
	        noModels: "This credential has no enabled activation model group.",
	        refresh: "Refresh credentials",
	        switchLanguage: "中文",
	        statusBusy: "Busy",
	        statusFailed: "Failed",
	        statusIdle: "Idle",
	        statusSkipped: "Skipped",
	        statusSuccess: "Success",
	        statusUnknown: "Unknown",
	        title: "Quota activation",
	        usable: "Available",
	        verifyKey: "Verify key"
	      }
	    };
	    let language = "zh-CN";
	    let authFiles = [];
	    function textFor(key) { return translations[language][key] || translations["zh-CN"][key] || key; }
	    function message(text) { document.getElementById("managementMessage").textContent = text; }
	    function managementKey() { const key = document.getElementById("managementKey").value.trim(); if (!key) { message(textFor("missingKey")); } return key; }
	    function statusText(raw) { const key = "status" + (raw || "idle").replace(/(^|_)([a-z])/g, (_, __, char) => char.toUpperCase()); return textFor(key) || textFor("statusUnknown"); }
	    function applyLanguage() { document.documentElement.lang = language; document.querySelectorAll("[data-i18n]").forEach((item) => { item.textContent = textFor(item.dataset.i18n); }); document.querySelectorAll("[data-i18n-placeholder]").forEach((item) => { item.placeholder = textFor(item.dataset.i18nPlaceholder); }); const statusValue = document.getElementById("currentStatusValue"); statusValue.textContent = statusText(statusValue.dataset.status); document.getElementById("languageSwitch").textContent = textFor("switchLanguage"); renderAuthFiles(); }
	    function switchLanguage() { language = language === "zh-CN" ? "en-US" : "zh-CN"; applyLanguage(); message(""); }
	    function shortError(defaultKey) { return textFor(defaultKey); }
	    async function managementFetch(path, options) { const key = managementKey(); if (!key) { return null; } const response = await fetch(path, { ...(options || {}), headers: { "Authorization": "Bearer " + key, "Content-Type": "application/json", ...((options && options.headers) || {}) } }); const text = await response.text(); if (!response.ok) { throw new Error(response.statusText || text); } return text ? JSON.parse(text) : {}; }
	    async function verifyManagementKey() { try { await managementFetch(STATUS_PATH, { method: "GET" }); document.getElementById("loginGate").hidden = true; document.getElementById("appShell").hidden = false; await loadAuthFiles(); message(textFor("keyPassed")); } catch (error) { message(shortError("keyFailed")); } }
	    async function loadAuthFiles() { const result = await managementFetch(AUTH_FILES_PATH, { method: "GET" }); if (!result) { return; } authFiles = Array.isArray(result.files) ? result.files : []; renderAuthFiles(); syncModelOptions(); if (authFiles.length === 0) { message(textFor("noAuthFiles")); } }
	    function renderAuthFiles() { const select = document.getElementById("credentialSelect"); if (!select) { return; } const selected = select.value; select.innerHTML = ""; for (const file of authFiles) { const option = document.createElement("option"); option.value = file.auth_id; option.textContent = [file.provider, file.label || file.auth_id, file.disabled ? textFor("disabled") : textFor("usable")].filter(Boolean).join(" · "); select.appendChild(option); } if (selected) { select.value = selected; } }
	    function syncModelOptions() { const credential = selectedCredential(); const modelSelect = document.getElementById("modelSelect"); modelSelect.innerHTML = ""; const models = credential && Array.isArray(credential.models) ? credential.models : []; for (const item of models) { const option = document.createElement("option"); option.value = item.value; option.dataset.group = item.group || ""; option.textContent = item.label || item.value; modelSelect.appendChild(option); } if (credential && models.length === 0) { message(textFor("noModels")); } }
	    function selectedCredential() { const authId = document.getElementById("credentialSelect").value; return authFiles.find((item) => item.auth_id === authId) || null; }
	    function buildManualActivationRequest() { const credential = selectedCredential(); if (!credential) { throw new Error(textFor("missingCredential")); } const modelOption = document.getElementById("modelSelect").selectedOptions[0]; if (!modelOption) { throw new Error(textFor("missingModel")); } return { auth_id: credential.auth_id, provider: credential.provider, model_group: modelOption.dataset.group || "", model: modelOption.value, disabled: Boolean(credential.disabled), quota_payload: credential.quota_payload }; }
	    async function triggerActivation() { try { const result = await managementFetch(ACTIVATE_PATH, { method: "POST", body: JSON.stringify(buildManualActivationRequest()) }); if (!result) { return; } message(textFor("activationDone")); } catch (error) { message(error.message === textFor("missingCredential") || error.message === textFor("missingModel") ? error.message : shortError("activationFailed")); } }
	    applyLanguage();
	  </script>
</body>
</html>`))
}
