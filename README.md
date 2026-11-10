<div align="center">

# Quota-Activation

[中文](./README.md) | [English](./README.en.md)

</div>

CLIProxyAPI (CPA) 配额唤醒插件。插件 ID、动态库基础名与 CPA 配置键均为 `quota-activation`。

## 导航

- [功能概览](#功能概览)
- [工作流程](#工作流程)
- [构建与安装](#构建与安装)
- [插件商店来源](#插件商店来源)
- [配置说明](#配置说明)
- [管理页面与接口](#管理页面与接口)
- [许可证](#许可证)

## 功能概览

- 支持 **Codex** 与 **Antigravity**（含 `gemini`、`claude_gpt` 与 `gemini_4_argon` 独立模型组）凭证的手动唤醒与自动扫描唤醒。
- 自动唤醒与手动唤醒逻辑解耦：自动扫描受周期硬跳闸限制，手动唤醒优先放行并支持即时触发。
- 自动额度窗口优先使用真实长窗口额度数据。
- Codex 自动解析凭据推断套餐类型，忽略 5 小时滚动短窗；缺少真实长窗口时，按套餐合成长窗口（付费 7 天，免费 / 未知 30 天）。
- Antigravity 使用 7 天默认长窗口，支持多组位置配对配置与凭证可用模型动态补全（基于能力提示扩展）。
- Antigravity 支持 `gemini-3.8-flash` 思考档位配置（`low` / `medium` / `high`），协议报文结构与客户端标识深度对齐官方规范。
- 优化手动激活放行与跳过判定：支持无周期数据凭据安全放行唤醒，模型分组隔离匹配杜绝跨组误判。
- 完善错误诊断与状态反馈：细分 429 配额耗尽与请求频控提示，精准识别 404 模型未开放错误，管理端全中文语义化展示。
- 管理页面支持中英双语、执行记录查看、手动唤醒成功/失败/跳过三态反馈与操作防重保护，全链路脱敏保障凭据安全。
- 批量激活与界面交互体验增强：前端模型选项兼容对象与字符串数组格式，凭证列表严格按提供商隔离筛选；批量唤醒时智能校验可用模型支持，自动跳过不支持的模型并统计为跳过，提升批量操作容错与稳定性。
- Antigravity 手动与自动唤醒均按凭证核对宿主已注册的模型，只使用该凭证实际可用且已启用的模型；自动扫描会过滤未注册模型，避免无效唤醒请求。
- 并发唤醒同一个已禁用凭证时会依次完成启用操作，避免凭证状态修改互相冲突；自动扫描记录按提供商汇总结果，并显示失败原因及次数，便于定位问题。
- 支持 **Antigravity** 与 **Codex** 的 OAuth 模型别名解析：唤醒配置与凭证模型支持别名映射，插件自动将别名还原为对应上游真实模型以进行模型组校验、额度探测与直连唤醒（仍仅匹配凭证实际已注册的模型或别名）；管理页面提供手动同步别名功能。
- 强化多凭证隔离与配额数据安全：管理端与自动扫描均会校验运行时凭证索引匹配，管理页面不再跨凭证借用模型列表，Antigravity 凭证在宿主未返回注册模型时置空可选模型与配额载荷；文件配额数据按提供商白名单清洗并剥离潜在敏感嵌套字段，Antigravity 缺省回退配额按模型组独立组装，杜绝跨组周期污染。

## 工作流程

```text
加载插件
  -> 读取 plugins.configs.quota-activation 配置
  -> 注册 management 路由与静态资源页 /status
  -> 手动：用户选择凭证与模型后 POST /activate
       - 凭证严格按活动提供商隔离筛选
       - 按单个凭证与提供商隔离加载模型及白名单清洗后的配额载荷（Antigravity 仅用宿主为该凭证注册的模型，回退周期按模型组独立组装）
       - 支持解析 Antigravity 与 Codex 的 OAuth 模型别名以匹配模型组与探测额度
       - 手动触发强制放行（绕过周期内已有成功的硬跳过限制）
       - 无周期数据凭据安全放行唤醒（不污染周期状态存储）
       - 批量激活智能校验可用模型并自动跳过不支持项
  -> 自动：auto_activate=true 时按 scan_interval 扫描
       - host.auth.list / get_runtime 取凭证并核对运行时凭证索引，防止跨凭证串用
       - 支持 OAuth 模型别名解析（匹配已注册模型/别名，并将 Antigravity 与 Codex 别名还原为上游真实模型用于校验权限、探测额度与直连唤醒）
       - 真实长窗口额度数据经提供商白名单清洗后优先使用
       - Codex 忽略 5 小时滚动短窗；无真实长窗时按套餐合成（付费 7 天，免费 / 未知 30 天）
        - Antigravity 保留 7 天默认长窗
        - Antigravity 候选按凭证已注册模型筛选，唤醒前确认目标模型可用
        - 同 cycle 已 success 则跳过（不以 5 小时短窗做硬跳过）
        - 与手动共用同一套唤醒内核
        - 默认 direct_http（host.http.do）；可选 scheduler_boost
        - 执行历史按提供商汇总扫描结果及失败原因次数
```

## 构建与安装

插件以 CGO 动态库形式运行，宿主会从动态库文件名去掉扩展名得到插件 ID，因此文件名必须保持为 `quota-activation.<ext>`。

```bash
CGO_ENABLED=1 go build -buildmode=c-shared -o quota-activation.so .
```

或：

```bash
./scripts/build.sh
```

把产物放入 CPA 插件发现目录之一：

- `plugins/<GOOS>/<GOARCH>/quota-activation.<ext>`
- `plugins/<GOOS>/<GOARCH>-<variant>/quota-activation.<ext>`
- `plugins/quota-activation.<ext>`

扩展名：Linux/FreeBSD 为 `.so`，macOS 为 `.dylib`，Windows 为 `.dll`。

## 插件商店来源

如需通过 CPA 插件商店安装本插件，第三方来源必须指向 `registry.json` 的原始 JSON 文本：

```yaml
plugins:
  enabled: true
  store-sources:
    - "https://raw.githubusercontent.com/Cody292/quota-activation/main/registry.json"
```

不要使用 `https://github.com/Cody292/quota-activation/blob/main/registry.json`。该地址返回 GitHub HTML 页面，CPA 无法按插件商店 registry 解析。修改 `store-sources` 后，重启 CPA 或通过管理端重新加载配置，再刷新插件商店列表。

## 配置说明

在 CPA `config.yaml` 中启用插件系统，并在 `plugins.configs.quota-activation` 下保留插件自有配置：

```yaml
plugins:
  enabled: true
  dir: "plugins"
  configs:
    quota-activation:
      enabled: true
      auto_activate: false
      enable_before_activation: false
      scan_interval: "30"
      activation_request_timeout: "60"
      max_concurrency: 1
      activation_prompt: "quota activation ping"
      # 默认 direct_http：经 host.http.do 直连上游，不写 priority / 不走调度器
      activation_transport: "direct_http"
      # direct_http 仅在传输/宿主类失败时回退 legacy scheduler_boost；业务失败不回退
      scheduler_boost_fallback: true
      # 宿主 OAuth 模型别名 JSON 映射（通常由管理页面手动同步写入，默认空字符串）
      oauth_model_aliases: ""
      activation_models:
        codex:
          models: "gpt-5.4-mini"
        antigravity:
          # 支持单组（如 "gemini" / "gemini-3-flash"）或 1~3 个模型组（用 | 分隔并按位置配对，如 "gemini|gemini_4_argon" 与 "gemini-3-flash|gemini-4-argon"；每个组只能一个模型，不支持数组/逗号）
          models_group: "gemini"
          models: "gemini-3-flash"
```

字段说明：

| 字段 | 说明 |
| :--- | :--- |
| `enabled` | 单插件开关；还需要全局 `plugins.enabled: true` 且动态库注册成功。 |
| `auto_activate` | 是否启用自动配额唤醒；默认 `false`，手动唤醒仍是默认方式。 |
| `enable_before_activation` | 为 `true` 时，达到唤醒条件后自动启用已禁用凭证并保持启用。 |
| `scan_interval` | 自动扫描间隔，**单位：分钟**。填写纯数字即可，无需带单位；默认 `30`。 |
| `activation_request_timeout` | 唤醒请求超时，**单位：秒**。填写纯数字即可；默认 `60`。 |
| `max_concurrency` | 自动扫描与激活的并发上限（worker 池大小）；默认 `1`。 |
| `activation_prompt` | 唤醒提示词（`direct_http` 写入上游请求体；`scheduler_boost` 经 `host.model.execute` 发送）。 |
| `activation_transport` | 唤醒传输方式：`direct_http`（**默认**，`host.http.do` 直连）或 `scheduler_boost`（legacy：临时提升 priority + `host.model.execute`）。 |
| `scheduler_boost_fallback` | 仅当 `activation_transport=direct_http` 时生效；默认 `true`。遇**传输/宿主类**失败可回退一次 `scheduler_boost`；**业务失败 / 假 2xx（无有效结构）禁止回退**，也不得写入 success cycle。 |
| `oauth_model_aliases` | 宿主 OAuth 模型别名的 JSON 映射字符串。用于将 Antigravity 与 Codex 的别名还原为上游真实模型；默认 `""`（空），同提供商内不允许空名称或模型名与别名冲突。通常由管理页面点击「同步 OAuth 模型别名」自动写入，无需手动维护。 |
| `activation_models.codex.models` | Codex 自动唤醒模型名称。 |
| `activation_models.antigravity.models_group` | Antigravity 自动唤醒模型组（`gemini`、`claude_gpt`、`gemini_4_argon`）。支持 1~3 个模型组并用 `\|` 分隔，与 `models` 按位置配对；每个组只能一个模型；旧单值仍兼容；不支持数组/逗号。 |
| `activation_models.antigravity.models` | Antigravity 自动唤醒模型名称。多组时同样用 `\|` 分隔并与 `models_group` 按位置配对；每个组只能一个模型；旧单值仍兼容；不支持数组/逗号。 |

说明：

- 仍可写带单位的 Go duration（如 `45m`、`2h`、`90s`）。
- 自动扫描与手动唤醒共用 `Activator.Activate` 内核；`scan_interval` 门闩允许约 2 秒微早触发，避免 timer 抖动静默丢轮。
- Antigravity 选用 `gemini-3.8-flash-{low,medium,high}` 时，插件会自动识别对应思考档位并配置生成参数。
- 字段 `activation_models`（`codex.models` 与 `antigravity.models`）支持填写已映射的别名，插件会自动还原为上游真实模型以匹配模型组与探测额度；其中 Antigravity 的 `models_group` 仍须对应上游真实模型所属分组。若同一上游模型在一个凭证上对应多个已注册别名，请直接填写目标已注册别名以免歧义。
- 凭证文件自带的 `quota_payload` 在探测和下发前会按提供商白名单清洗，剥离无关嵌套与潜在敏感字段；缺失真实文件配额时，Antigravity 按模型组独立组装回退周期，避免跨组污染。
- 管理端页面、诊断信息与执行历史的失败原因均为**纯中文**（针对上游 429 额度耗尽、404 模型未开放等具体场景提供精准语义反馈）。

## 管理页面与接口

### 资源页面

- `GET /v0/resource/plugins/quota-activation/status`
  返回静态 HTML 页面，用于管理密钥验证、凭证选择与手动唤醒。页面内动态请求走 management API。
  - **同步 OAuth 模型别名（操作步骤）**：
    1. 打开管理页面并输入 CPA 管理密钥完成验证。
    2. 点击「同步 OAuth 模型别名」按钮，页面会从宿主拉取 Antigravity 与 Codex 当前的别名列表并写入 `oauth_model_aliases` 配置。
    3. 等待 CPA 宿主完成配置重载（热重载或重启宿主）后，点击「刷新凭证」（或查看诊断接口）即可按新别名加载模型分组与发起唤醒。
  - **手动同步与重载说明**：别名同步为按需手动单次操作，插件不会在后台自动持续同步；当宿主中的 OAuth 模型别名新增或变更后，需再次点击同步。同步成功提示仅表示已即时写入插件配置，运行时生效仍以 CPA 宿主完成配置重载为准。
  - **多凭证模型隔离**：管理页面按单个凭证名称与提供商独立加载可用模型，不再向其他凭证借用模型列表；若 Antigravity 凭证在宿主侧未返回已注册模型，界面将显示无可用模型组并留空配额载荷。

### 管理 API

以下插件管理接口需要 CPA 管理密钥：

- `GET /v0/management/quota-activation/status`
- `GET /v0/management/quota-activation/auth-files`
- `POST /v0/management/quota-activation/activate`
- `GET /v0/management/quota-activation/diagnostics`（返回当前运行态已应用且脱敏后的 `oauth_model_aliases` 与应用状态）

管理页面同时调用以下 CPA 宿主接口：

- `GET /v0/management/auth-files/models?name=...`（查询单个凭证可用模型列表）
- `GET /v0/management/oauth-model-alias`（读取宿主当前的 Antigravity 与 Codex OAuth 模型别名）
- `PATCH /v0/management/plugins/quota-activation/config`（将同步得到的 `oauth_model_aliases` 写入插件配置）

## 许可证

本项目使用 MIT License，详见 [LICENSE](./LICENSE)。
