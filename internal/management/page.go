package management

import (
	"net/http"
)

// allow: SIZE_OK - 内嵌静态资源页以单 HTML 载荷交给 CPA resources 路由；拆分不会降低运行时职责。
func (h *Handler) handlePage(w http.ResponseWriter) {
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
    section { position: relative; background: #fff; border: 1px solid #e5e7eb; border-radius: 16px; padding: 24px; box-shadow: 0 12px 30px rgba(15, 23, 42, .08); }
    h1 { margin: 0; font-size: 24px; white-space: nowrap; display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
    .version-badge { display: inline-flex; align-items: center; min-height: 24px; border-radius: 999px; padding: 3px 9px; background: #eff6ff; color: #1d4ed8; font-size: 12px; font-weight: 750; letter-spacing: 0; }
    p { line-height: 1.7; color: #4b5563; }
    code { background: #f3f4f6; border-radius: 6px; padding: 2px 6px; }
    label { display: block; margin: 18px 0 8px; color: #374151; font-weight: 600; }
    input, select { width: 100%; box-sizing: border-box; border: 1px solid #d1d5db; border-radius: 10px; padding: 12px; font: inherit; }
    .header-bar { display: flex; align-items: center; justify-content: space-between; gap: 16px; margin-bottom: 24px; }
    .search-container { flex: 1; display: flex; justify-content: center; }
    .search-container input { width: 100%; max-width: 280px; padding: 10px 14px; border-radius: 10px; border: 1px solid #d1d5db; font-size: 14px; }
    .top-actions { display: flex; align-items: center; gap: 12px; }
    .language-shell { position: relative; display: inline-flex; align-items: center; }
    .language-menu-button { min-height: 40px; display: inline-flex; align-items: center; gap: 8px; border: 1px solid #d1d5db; border-radius: 999px; padding: 0 14px; background: #fff; color: #374151; font-size: 14px; font-weight: 700; box-shadow: 0 1px 2px rgba(15, 23, 42, .06); }
    .language-menu-button svg { width: 16px; height: 16px; }
    .actions { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 12px; margin-top: 16px; }
    .message { min-height: 24px; margin-top: 14px; color: #374151; white-space: pre-wrap; }
    .toast { position: absolute; top: 24px; right: 24px; z-index: 1000; max-width: min(360px, calc(100vw - 48px)); margin: 0; padding: 12px 16px; border-radius: 12px; background: #111827; color: #fff; box-shadow: 0 18px 40px rgba(15, 23, 42, .22); pointer-events: none; }
    .toast:empty { display: none; }
    button { min-height: 44px; border: 0; border-radius: 10px; padding: 0 18px; background: #111827; color: #fff; font-weight: 600; cursor: pointer; }
    button.secondary { background: #e5e7eb; color: #111827; }
    button:disabled { opacity: .6; cursor: not-allowed; }

    .select-shell { border-radius: 10px; border: 1px solid #d1d5db; background: #fff; overflow: hidden; }
    .credential-list { max-height: 220px; overflow: auto; padding: 8px; display: flex; flex-direction: column; gap: 4px; }
    .credential-item { display: flex; align-items: center; gap: 10px; margin: 0; padding: 8px 10px; border-radius: 8px; font-weight: 500; color: #374151; cursor: pointer; }
    .credential-item:hover { background: #f3f4f6; }
    .credential-item.disabled { color: #9ca3af; cursor: not-allowed; }
    .credential-item.disabled:hover { background: transparent; }
    .credential-item input { width: auto; margin: 0; }
    .credential-item.disabled input { pointer-events: none; }
    .model-menu { position: relative; }
    .model-menu-button { width: 100%; min-height: 44px; display: flex; align-items: center; justify-content: space-between; gap: 12px; border: 1px solid #d1d5db; border-radius: 10px; padding: 0 14px; background: #fff; color: #111827; font: inherit; font-weight: 500; text-align: left; cursor: pointer; box-shadow: 0 1px 2px rgba(15, 23, 42, .04); }
    .model-menu-button:disabled { opacity: .6; cursor: not-allowed; }
    .model-menu-panel { position: absolute; left: 0; right: 0; top: calc(100% + 6px); z-index: 40; display: none; max-height: 220px; overflow: auto; border: 1px solid #d1d5db; border-radius: 10px; background: #fff; box-shadow: 0 16px 36px rgba(15, 23, 42, .14); padding: 6px; }
    .model-menu-panel.open { display: block; }
    .model-option { width: 100%; min-height: 40px; display: block; border: 0; border-radius: 8px; padding: 8px 12px; background: transparent; color: #111827; font: inherit; text-align: left; cursor: pointer; }
    .model-option:hover, .model-option.active { background: #f3f4f6; }
    .model-option.disabled { color: #9ca3af; cursor: not-allowed; opacity: .55; }
    .model-option.disabled:hover { background: transparent; }
    .provider-tabs { display: flex; flex-wrap: wrap; gap: 8px; margin: 0 0 12px; }
    .provider-tab { min-height: 36px; border: 1px solid #d1d5db; border-radius: 999px; padding: 0 14px; background: #fff; color: #374151; font-weight: 600; cursor: pointer; }
    .provider-tab.active { background: #111827; color: #fff; border-color: #111827; }

    .tabs { display: flex; gap: 8px; margin-bottom: 20px; border-bottom: 1px solid #e5e7eb; padding-bottom: 8px; }
    .tab-button { background: transparent; color: #4b5563; font-weight: 500; min-height: 38px; padding: 0 16px; border-radius: 8px; border: 1px solid transparent; cursor: pointer; }
    .tab-button:hover { background: #f3f4f6; }
    .tab-button.active { background: #111827; color: #fff; font-weight: 600; }
    .tab-panel { display: none; }
    .tab-panel.active { display: block; }
    .help-content { line-height: 1.6; color: #374151; }
    .help-content h3 { margin-top: 20px; margin-bottom: 8px; color: #111827; }
    .help-content ul { padding-left: 20px; margin-bottom: 16px; }
    .help-content li { margin-bottom: 6px; }
    .help-content pre { background: #f3f4f6; padding: 12px; border-radius: 8px; overflow-x: auto; font-family: monospace; font-size: 14px; margin: 8px 0; }
  </style>
</head>
<body>
	  <main>
	    <section>
	      <div id="loginGate">
	        <input id="managementKey" type="password" autocomplete="current-password" data-i18n-placeholder="managementKeyPlaceholder" placeholder="输入 CPA 管理密钥">
	        <div class="actions"><button type="button" data-i18n="verifyKey" onclick="verifyManagementKey(this)">验证密钥</button></div>
	      </div>
	      <div id="appShell" hidden>
	        <div class="header-bar">
	          <h1><span data-i18n="title">配额唤醒</span><span class="version-badge">v0.0.2</span></h1>
	          <div class="search-container">
	            <input id="credentialSearch" type="search" data-i18n-placeholder="searchPlaceholder" placeholder="搜索凭证..." oninput="renderAuthFiles()">
	          </div>
	          <div class="top-actions">
	            <div class="language-shell">
	              <button id="languageMenuButton" type="button" class="language-menu-button" onclick="switchLanguage()" aria-label="Switch language">
	                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
	                  <circle cx="12" cy="12" r="10"></circle>
	                  <path d="M2 12h20"></path>
	                  <path d="M12 2a15.3 15.3 0 0 1 0 20"></path>
	                  <path d="M12 2a15.3 15.3 0 0 0 0 20"></path>
	                </svg>
	              </button>
	            </div>
	          </div>
	        </div>
	        <p id="managementToast" class="toast" role="status"></p>

	        <div class="tabs">
	          <button id="manualTabButton" class="tab-button active" onclick="switchTab('manual')" data-i18n="manualTab">手动唤醒</button>
	          <button id="helpTabButton" class="tab-button" onclick="switchTab('help')" data-i18n="helpTab">帮助</button>
	        </div>

	        <div id="manualPanel" class="tab-panel active">
	          <label for="credentialSelect" data-i18n="credential">凭证</label>
	          <div id="providerTabs" class="provider-tabs"></div>
	          <div id="credentialSelect" class="select-shell credential-list" role="group" aria-label="credentials"></div>
	          <label for="modelMenuButton" data-i18n="model">唤醒模型</label>
	          <div class="model-menu" id="modelSelect">
	            <button id="modelMenuButton" type="button" class="model-menu-button" onclick="toggleModelMenu()" aria-haspopup="listbox" aria-expanded="false">
	              <span id="modelMenuLabel">—</span>
	              <span aria-hidden="true">▾</span>
	            </button>
	            <div id="modelMenuPanel" class="model-menu-panel" role="listbox"></div>
	          </div>
	          <div class="actions">
	            <button id="refreshCredentialsButton" type="button" class="secondary" data-i18n="refresh" onclick="loadAuthFiles(this)">刷新凭证</button>
	            <button id="manualActivateButton" type="button" data-i18n="activate" onclick="triggerActivation(this)">手动触发唤醒</button>
	          </div>
	        </div>

	        <div id="helpPanel" class="tab-panel">
	          <div class="help-content">
	            <h3 data-i18n="helpConfigTitle">配置字段说明</h3>
	            <p data-i18n="helpConfigDesc">配置 quota-activation 插件所需的字段如下：</p>
	            <ul>
	              <li><code>auto_activate</code>: <span data-i18n="helpAutoActivate">是否开启自动配额唤醒。</span></li>
	              <li><code>enable_before_activation</code>: <span data-i18n="helpEnableBefore">是否在唤醒前自动启用已被禁用的凭证。</span></li>
	              <li><code data-optional="true">scan_interval</code>: <span data-i18n="helpScanInterval">可选；自动扫描间隔，单位分钟，填写纯数字即可（默认 30）。</span></li>
	              <li><code data-optional="true">activation_request_timeout</code>: <span data-i18n="helpActivationTimeout">可选；唤醒请求超时，单位秒，填写纯数字即可（默认 60）。</span></li>
	              <li><code data-optional="true">max_concurrency</code>: <span data-i18n="helpMaxConcurrency">可选；最大并发唤醒请求数。</span></li>
	              <li><code data-optional="true">activation_prompt</code>: <span data-i18n="helpActivationPrompt">可选；唤醒提示词。</span></li>
	            </ul>
	            <h3 data-i18n="helpActivationModelsTitle">自动唤醒模型字段</h3>
	            <p data-i18n="helpActivationModelsDesc">直接填写模型名称，不需要填写 JSON 对象：</p>
	            <ul>
	              <li><code>activation_models.codex.models</code>: <span data-i18n="helpCodexModels">Codex 自动唤醒模型名称，例如 <code>gpt-5-mini</code>。</span></li>
	              <li><code>activation_models.antigravity.models_group</code>: <span data-i18n="helpAntigravityGroup">Antigravity 自动唤醒模型组，可选 <code>gemini</code> 或 <code>claude_gpt</code>。</span></li>
	              <li><code>activation_models.antigravity.models</code>: <span data-i18n="helpAntigravityModels">当前 Antigravity 模型组的模型名称。</span></li>
	            </ul>
	          </div>
	        </div>
	      </div>
	    </section>
	  </main>
	  <script>
	    const STATUS_PATH = "/v0/management/quota-activation/status";
	    const AUTH_FILES_PATH = "/v0/management/quota-activation/auth-files";
	    const AUTH_FILE_MODELS_PATH = "/v0/management/auth-files/models";
	    const ACTIVATE_PATH = "/v0/management/quota-activation/activate";
	    const translations = {
	      "zh-CN": {
	        activate: "手动触发唤醒",
	        activationDone: "手动唤醒请求已完成。",
	        activationFailed: "手动唤醒失败，请稍后重试或检查凭证状态。",
	        statusFailed: "唤醒失败",
	        activationRunning: "唤醒中...",
	        credential: "凭证",
	        disabled: "已禁用",
	        credentialDisabled: "请先启用凭证后再尝试唤醒",
	        loadingCredentials: "刷新中...",
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
	        statusBusy: "执行中",
	        statusSkipped: "已跳过",
	        statusSuccess: "成功",
	        statusUnknown: "未知",
	        title: "配额唤醒",
	        usable: "可用",
	        verifyKey: "验证密钥",
	        verifyingKey: "验证中...",
	        manualTab: "手动唤醒",
	        helpTab: "帮助",
	        searchPlaceholder: "搜索凭证...",
	        helpConfigTitle: "配置字段说明",
	        helpConfigDesc: "配置 quota-activation 插件所需的字段如下：",
	        helpActivationModelsTitle: "自动唤醒模型字段",
	        helpActivationModelsDesc: "直接填写模型名称，不需要填写 JSON 对象：",
	        helpAutoActivate: "是否开启自动配额唤醒。",
	        helpEnableBefore: "是否在唤醒前自动启用已被禁用的凭证。",
	        helpScanInterval: "可选；自动扫描间隔，单位分钟，填写纯数字即可（默认 30）。",
	        helpActivationTimeout: "可选；唤醒请求超时，单位秒，填写纯数字即可（默认 60）。",
	        helpMaxConcurrency: "可选；最大并发唤醒请求数。",
	        helpActivationPrompt: "可选；唤醒提示词。",
	        helpCodexModels: "Codex 自动唤醒模型名称，例如 gpt-5-mini。",
	        helpAntigravityGroup: "Antigravity 自动唤醒模型组，可选 gemini 或 claude_gpt。",
	        helpAntigravityModels: "当前 Antigravity 模型组的模型名称。"
	      },
	      "en-US": {
	        activate: "Trigger activation",
	        activationDone: "Manual activation request completed.",
	        activationFailed: "Manual activation failed. Try again later or check the credential status.",
	        statusFailed: "Activation failed",
	        activationRunning: "Activating...",
	        credential: "Credential",
	        disabled: "Disabled",
	        credentialDisabled: "Enable the credential before trying to wake it up.",
	        loadingCredentials: "Refreshing...",
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
	        statusBusy: "Busy",
	        statusSkipped: "Skipped",
	        statusSuccess: "Success",
	        statusUnknown: "Unknown",
	        title: "Quota activation",
	        usable: "Available",
	        verifyKey: "Verify key",
	        verifyingKey: "Verifying...",
	        manualTab: "Manual Activation",
	        helpTab: "Help",
	        searchPlaceholder: "Search credentials...",
	        helpConfigTitle: "Configuration Fields",
	        helpConfigDesc: "The fields required to configure the quota-activation plugin are as follows:",
	        helpActivationModelsTitle: "Automatic activation model fields",
	        helpActivationModelsDesc: "Enter model names directly; no JSON object is required:",
	        helpAutoActivate: "Whether to enable automatic quota activation.",
	        helpEnableBefore: "Whether to automatically enable disabled credentials before activation.",
	        helpScanInterval: "Optional; automatic scan interval in minutes. Enter a number only (default 30).",
	        helpActivationTimeout: "Optional; activation request timeout in seconds. Enter a number only (default 60).",
	        helpMaxConcurrency: "Optional; maximum concurrent activation requests.",
	        helpActivationPrompt: "Optional; activation prompt text.",
	        helpCodexModels: "Codex automatic activation model name, for example gpt-5-mini.",
	        helpAntigravityGroup: "Antigravity automatic activation model group: gemini or claude_gpt.",
	        helpAntigravityModels: "Model name for the current Antigravity model group."
	      }
	    };
	    let language = "zh-CN";
	    let authFiles = [];
	    let activeProvider = "";
	    let toastTimer = 0;
	    function textFor(key) { return translations[language][key] || translations["zh-CN"][key] || key; }
	    function message(text) { const toast = document.getElementById("managementToast"); toast.textContent = text; if (toastTimer) { clearTimeout(toastTimer); } if (text) { toastTimer = setTimeout(() => { toast.textContent = ""; toastTimer = 0; }, 2500); } else { toastTimer = 0; } }
	    function managementKey() { const key = document.getElementById("managementKey").value.trim(); if (!key) { message(textFor("missingKey")); } return key; }
	    function applyLanguage() { document.documentElement.lang = language; document.querySelectorAll("[data-i18n]").forEach((item) => { item.textContent = textFor(item.dataset.i18n); }); document.querySelectorAll("[data-i18n-placeholder]").forEach((item) => { item.placeholder = textFor(item.dataset.i18nPlaceholder); }); renderAuthFiles(); }
	    function switchLanguage() { language = language === "zh-CN" ? "en-US" : "zh-CN"; applyLanguage(); message(""); }
	    function shortError(defaultKey) { return textFor(defaultKey); }
	    function setButtonBusy(control, labelKey) { if (!control) { return null; } const state = { control, labelKey: control.dataset.i18n || "", text: control.textContent }; control.disabled = true; control.textContent = textFor(labelKey); return state; }
	    function restoreButton(state) { if (!state) { return; } state.control.disabled = false; state.control.textContent = state.labelKey ? textFor(state.labelKey) : state.text; }
	    function activationErrorMessage(error) { const raw = String(error && error.message ? error.message : error); try { const payload = JSON.parse(raw); return payload.last_error || payload.message || raw || textFor("activationFailed"); } catch (_) { return raw || textFor("activationFailed"); } }
	    async function managementFetch(path, options) { const key = managementKey(); if (!key) { return null; } const response = await fetch(path, { ...(options || {}), headers: { "Authorization": "Bearer " + key, "Content-Type": "application/json", ...((options && options.headers) || {}) } }); const text = await response.text(); if (!response.ok) { throw new Error(text || response.statusText); } return text ? JSON.parse(text) : {}; }
	    async function verifyManagementKey(button) { const buttonState = setButtonBusy(button, "verifyingKey"); try { await managementFetch(STATUS_PATH, { method: "GET" }); document.getElementById("loginGate").hidden = true; document.getElementById("appShell").hidden = false; await loadAuthFiles(); message(textFor("keyPassed")); } catch (error) { message(shortError("keyFailed")); } finally { restoreButton(buttonState); } }
	    function setManualActionBusy(busy, labelKey) {
	      const refreshBtn = document.getElementById("refreshCredentialsButton");
	      const activateBtn = document.getElementById("manualActivateButton");
	      const state = { refresh: null, activate: null };
	      if (busy) {
	        if (refreshBtn) {
	          state.refresh = { disabled: refreshBtn.disabled, text: refreshBtn.textContent, labelKey: refreshBtn.dataset.i18n || "refresh" };
	          refreshBtn.disabled = true;
	          if (labelKey) { refreshBtn.textContent = textFor(labelKey); }
	        }
	        if (activateBtn) {
	          state.activate = { disabled: activateBtn.disabled, text: activateBtn.textContent, labelKey: activateBtn.dataset.i18n || "activate" };
	          activateBtn.disabled = true;
	        }
	      }
	      return state;
	    }
	    function restoreManualActionBusy(state) {
	      if (!state) { return; }
	      const refreshBtn = document.getElementById("refreshCredentialsButton");
	      const activateBtn = document.getElementById("manualActivateButton");
	      if (refreshBtn && state.refresh) {
	        refreshBtn.disabled = Boolean(state.refresh.disabled);
	        refreshBtn.textContent = state.refresh.labelKey ? textFor(state.refresh.labelKey) : state.refresh.text;
	      }
	      if (activateBtn && state.activate) {
	        activateBtn.disabled = Boolean(state.activate.disabled);
	        activateBtn.textContent = state.activate.labelKey ? textFor(state.activate.labelKey) : state.activate.text;
	      }
	    }
	    async function loadAuthFiles(button) {
	      void button;
	      const manualBusy = setManualActionBusy(true, "loadingCredentials");
	      try {
	        const result = await managementFetch(AUTH_FILES_PATH, { method: "GET" });
	        if (!result) { return; }
	        authFiles = Array.isArray(result.files) ? result.files : [];
	        await loadCredentialModels();
	        renderAuthFiles();
	        syncModelOptions();
	        if (authFiles.length === 0) { message(textFor("noAuthFiles")); }
	      } catch (error) {
	        message(activationErrorMessage(error));
	      } finally {
	        restoreManualActionBusy(manualBusy);
	      }
	    }
	    async function loadCredentialModels() {
	      const providerCache = {};
	      const pendingByProvider = {};
	      for (const credential of authFiles) {
	        if (!credential) { continue; }
	        if (Array.isArray(credential.models) && credential.models.length > 0) { continue; }
	        const provider = String(credential.provider || "").toLowerCase();
	        const cacheKey = provider || ("name:" + String(credential.name || credential.auth_id || ""));
	        if (!credential.name && !providerCache[cacheKey]) { credential.models = Array.isArray(credential.models) ? credential.models : []; continue; }
	        try {
	          if (!Object.prototype.hasOwnProperty.call(providerCache, cacheKey)) {
	            if (!pendingByProvider[cacheKey]) {
	              const sampleName = credential.name || (authFiles.find((item) => item && item.name && String(item.provider || "").toLowerCase() === provider) || {}).name;
	              pendingByProvider[cacheKey] = sampleName
	                ? managementFetch(AUTH_FILE_MODELS_PATH + "?name=" + encodeURIComponent(sampleName), { method: "GET" }).then((result) => Array.isArray(result && result.models) ? result.models : []).catch(() => [])
	                : Promise.resolve([]);
	            }
	            providerCache[cacheKey] = await pendingByProvider[cacheKey];
	          }
	          credential.models = modelChoicesForCredential(credential, providerCache[cacheKey] || []);
	        } catch (_) {
	          credential.models = Array.isArray(credential.models) ? credential.models : [];
	        }
	      }
	    }
	    function modelChoicesForCredential(credential, models) { const choices = []; for (const item of models) { const value = String((item && (item.id || item.name || item.value)) || "").trim(); if (!value) { continue; } if (credential.provider === "antigravity") { const group = antigravityModelGroup(value); if (!group) { continue; } choices.push({ value, label: (group === "gemini" ? "Gemini" : "Claude/GPT") + " · " + value, group }); } else { choices.push({ value, label: "Codex · " + value }); } } return choices; }
	    function antigravityModelGroup(model) { const lower = String(model || "").toLowerCase(); if (lower.includes("gemini")) { return "gemini"; } if (lower.includes("claude") || lower.includes("gpt")) { return "claude_gpt"; } return ""; }
	    function switchTab(tab) {
	      document.getElementById("manualTabButton").classList.toggle("active", tab === "manual");
	      document.getElementById("helpTabButton").classList.toggle("active", tab === "help");
	      document.getElementById("manualPanel").classList.toggle("active", tab === "manual");
	      document.getElementById("helpPanel").classList.toggle("active", tab === "help");
	    }
	    function providerLabel(key) {
	      if (key === "antigravity") return "Antigravity";
	      if (key === "codex") return "Codex";
	      return language === "zh-CN" ? "其他" : "Other";
	    }
	    function normalizeProviderKey(provider) {
	      const key = String(provider || "").toLowerCase();
	      if (key === "antigravity" || key === "codex") return key;
	      return "other";
	    }
	    function credentialProvider(file) {
	      return file ? file["provider"] : "";
	    }
	    function availableProviders() {
	      const seen = new Set();
	      const order = ["antigravity", "codex", "other"];
	      for (const file of authFiles) {
	        seen.add(normalizeProviderKey(credentialProvider(file)));
	      }
	      return order.filter((key) => seen.has(key));
	    }
	    function switchProvider(provider) {
	      activeProvider = provider || "";
	      renderAuthFiles();
	      syncModelOptions();
	    }
	    function renderProviderTabs(providers) {
	      const tabs = document.getElementById("providerTabs");
	      if (!tabs) { return; }
	      tabs.innerHTML = "";
	      for (const key of providers) {
	        const button = document.createElement("button");
	        button.type = "button";
	        button.className = "provider-tab" + (key === activeProvider ? " active" : "");
	        button.dataset.provider = key;
	        button.textContent = providerLabel(key);
	        button.onclick = () => switchProvider(key);
	        tabs.appendChild(button);
	      }
	    }
	    let selectedModel = null;
	    function renderAuthFiles() {
	      const list = document.getElementById("credentialSelect");
	      if (!list) { return; }
	      const providers = availableProviders();
	      if (!activeProvider || !providers.includes(activeProvider)) {
	        activeProvider = providers.length > 0 ? providers[0] : "";
	      }
	      renderProviderTabs(providers);
	      const searchVal = (document.getElementById("credentialSearch")?.value || "").toLowerCase().trim();
	      const selectedValues = Array.from(list.querySelectorAll('input[type="checkbox"]:checked')).map((input) => input.value);
	      list.innerHTML = "";
	      const filtered = authFiles.filter(file => {
	        if (normalizeProviderKey(credentialProvider(file)) !== activeProvider) return false;
	        if (!searchVal) return true;
	        const label = (file.label || "").toLowerCase();
	        const authId = (file.auth_id || "").toLowerCase();
	        return label.includes(searchVal) || authId.includes(searchVal);
	      });
	      for (const file of filtered) {
	        const row = document.createElement("label");
	        row.className = "credential-item";
	        row.classList.toggle("disabled", Boolean(file.disabled));
	        row.onclick = () => {
	          if (!file.disabled) { return; }
	          message(textFor("credentialDisabled"));
	        };
	        const checkbox = document.createElement("input");
	        checkbox.type = "checkbox";
	        checkbox.value = file.auth_id;
	        checkbox.disabled = Boolean(file.disabled);
	        checkbox.checked = !file.disabled && selectedValues.includes(file.auth_id);
	        checkbox.onchange = () => syncModelOptions();
	        const text = document.createElement("span");
	        text.textContent = [file.label || file.auth_id, file.disabled ? textFor("disabled") : textFor("usable")].filter(Boolean).join(" · ");
	        row.appendChild(checkbox);
	        row.appendChild(text);
	        list.appendChild(row);
	      }
	    }
	    function setModelMenuOpen(open) {
	      const panel = document.getElementById("modelMenuPanel");
	      const button = document.getElementById("modelMenuButton");
	      if (!panel || !button) { return; }
	      panel.classList.toggle("open", !!open);
	      button.setAttribute("aria-expanded", open ? "true" : "false");
	    }
	    function toggleModelMenu() {
	      const panel = document.getElementById("modelMenuPanel");
	      if (!panel) { return; }
	      setModelMenuOpen(!panel.classList.contains("open"));
	    }
	    function chooseModel(value, group, label) {
	      selectedModel = value ? { value, group: group || "", label: label || value } : null;
	      const modelLabel = document.getElementById("modelMenuLabel");
	      if (modelLabel) {
	        modelLabel.textContent = selectedModel ? selectedModel.label : "—";
	      }
	      const panel = document.getElementById("modelMenuPanel");
	      if (panel) {
	        panel.querySelectorAll(".model-option").forEach((btn) => {
	          btn.classList.toggle("active", !!(selectedModel && btn.dataset.value === selectedModel.value));
	        });
	      }
	      setModelMenuOpen(false);
	    }
	    function syncModelOptions() {
	      const credentials = selectedCredentials();
	      const panel = document.getElementById("modelMenuPanel");
	      if (!panel) { return; }
	      const selectedVal = selectedModel ? selectedModel.value : "";
	      panel.innerHTML = "";
	      const seen = new Set();
	      const choices = [];
	      for (const credential of credentials) {
	        const models = credential && Array.isArray(credential.models) ? credential.models : [];
	        for (const item of models) {
	          if (!seen.has(item.value)) {
	            seen.add(item.value);
	            choices.push(item);
	          }
	        }
	      }
	      const selectedGroup = selectedModel ? selectedModel.group : "";
	      for (const item of choices) {
	        const option = document.createElement("button");
	        option.type = "button";
	        option.disabled = Boolean(selectedGroup && item.group && item.group !== selectedGroup);
	        option.className = "model-option" + (item.value === selectedVal ? " active" : "") + (option.disabled ? " disabled" : "");
	        option.dataset.value = item.value;
	        option.dataset.group = item.group || "";
	        option.textContent = item.label || item.value;
	        option.onclick = () => { if (option.disabled) { return; } chooseModel(item.value, item.group || "", item.label || item.value); };
	        panel.appendChild(option);
	      }
	      if (choices.length === 0) {
	        chooseModel("", "", "—");
	      } else if (!choices.some((item) => item.value === selectedVal)) {
	        chooseModel(choices[0].value, choices[0].group || "", choices[0].label || choices[0].value);
	      } else {
	        const current = choices.find((item) => item.value === selectedVal);
	        chooseModel(current.value, current.group || "", current.label || current.value);
	      }
	      if (credentials.length > 0 && choices.length === 0) {
	        const hasEnabled = credentials.some((credential) => credential && !credential.disabled);
	        if (hasEnabled) {
	          message(textFor("noModels"));
	        }
	      }
	    }
	    function selectedCredentials() {
	      const list = document.getElementById("credentialSelect");
	      if (!list) return [];
	      const selectedValues = Array.from(list.querySelectorAll('input[type="checkbox"]:checked')).map((input) => input.value);
	      return authFiles.filter((item) => selectedValues.includes(item.auth_id));
	    }
	    function selectedCredential() {
	      const list = selectedCredentials();
	      return list.length > 0 ? list[0] : null;
	    }
	    function selectedModelOption() {
	      if (!selectedModel || !selectedModel.value) { return null; }
	      return { value: selectedModel.value, dataset: { group: selectedModel.group || "" } };
	    }
	    function quotaPayloadForActivation(credential, modelOption) { if (!credential || credential.provider !== "antigravity") { return credential ? credential.quota_payload : {}; } const model = modelOption.value; const group = modelOption.dataset.group || antigravityModelGroup(model); const provider = group === "gemini" ? "gemini" : "anthropic"; return { models: { [model]: { modelProvider: provider, quotaInfo: { windows: [{ resetTime: new Date(Date.now() + 5 * 60 * 60 * 1000).toISOString(), name: "5h" }] } } } }; }
	    async function triggerActivation(button) {
	      const credentials = selectedCredentials();
	      if (credentials.length === 0) {
	        message(textFor("missingCredential"));
	        return;
	      }
	      const modelOption = selectedModelOption();
	      if (!modelOption) {
	        message(textFor("missingModel"));
	        return;
	      }
	      const actBtn = button || document.getElementById("manualActivateButton");
	      const refBtn = document.getElementById("refreshCredentialsButton");
	      const actState = setButtonBusy(actBtn, "activationRunning");
	      let refBtnDisabledState = false;
	      if (refBtn) {
	        refBtnDisabledState = refBtn.disabled;
	        refBtn.disabled = true;
	      }
	      try {
	        let successCount = 0;
	        let failedCount = 0;
	        let lastError = null;
	        for (const credential of credentials) {
	          const payload = {
	            auth_id: credential.auth_id,
	            provider: credential.provider,
	            model_group: modelOption.dataset.group || "",
	            model: modelOption.value,
	            disabled: Boolean(credential.disabled),
	            quota_payload: quotaPayloadForActivation(credential, modelOption)
	          };
	          try {
	            await managementFetch(ACTIVATE_PATH, {
	              method: "POST",
	              body: JSON.stringify(payload)
	            });
	            successCount++;
	          } catch (error) {
	            failedCount++;
	            lastError = error;
	          }
	        }
	        if (failedCount === 0) {
	          message(textFor("activationDone"));
	        } else {
	          if (lastError && (lastError.message === textFor("missingCredential") || lastError.message === textFor("missingModel"))) {
	            message(lastError.message);
	          } else {
	            message(activationErrorMessage(lastError));
	          }
	        }
	      } catch (error) {
	        message(activationErrorMessage(error));
	      } finally {
	        restoreButton(actState);
	        if (refBtn) {
	          refBtn.disabled = refBtnDisabledState;
	        }
	      }
	    }
	    document.addEventListener("click", (event) => {
	      const menu = document.getElementById("modelSelect");
	      if (menu && !menu.contains(event.target)) {
	        setModelMenuOpen(false);
	      }
	    });
	    applyLanguage();
	  </script>
</body>
</html>`))
}
