<div align="center">

# Quota-Activation

[中文](./README.md) | [English](./README.en.md)

</div>

CLIProxyAPI (CPA) 配额唤醒插件。插件 ID、动态库基础名与 CPA 配置键均为 `quota-activation`。

当前版本：`v0.0.1`

## 导航

- [功能概览](#功能概览)
- [工作流程](#工作流程)
- [构建与安装](#构建与安装)
- [插件商店来源](#插件商店来源)
- [配置说明](#配置说明)
- [管理页面与接口](#管理页面与接口)
- [许可证](#许可证)

## 功能概览

- 支持 **Codex** 与 **Antigravity** 凭证的手动唤醒与自动扫描唤醒。
- 自动唤醒与手动唤醒配置分离：自动路径仅使用 `auto_activate`、`scan_interval`、`activation_models.*`。
- 自动额度窗口优先使用真实 `quota_payload`；缺失时按历史成功记录推断，再回退 provider 默认长窗。
- 管理页面与诊断接口只展示脱敏信息，不输出 token / 密钥。

## 工作流程

```text
加载插件
  -> 读取 plugins.configs.quota-activation 配置
  -> 注册 management 路由与静态资源页 /status
  -> 手动：用户选择凭证与模型后 POST /activate
  -> 自动：auto_activate=true 时按 scan_interval 扫描
       - host.auth.list / get_runtime 取凭证
       - 真实额度 payload 优先，否则 state 推断，再兜底长窗
       - 同 cycle 已 success 则跳过
       - 通过 host.model.execute 发送唤醒请求
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
      activation_models:
        codex:
          models: "gpt-5-mini"
        antigravity:
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
| `max_concurrency` | 最大并发唤醒请求数；当前流程预期为 `1`。 |
| `activation_prompt` | 通过 `host.model.execute` 发送的配额唤醒提示词。 |
| `activation_models.codex.models` | Codex 自动唤醒模型名称。 |
| `activation_models.antigravity.models_group` | Antigravity 自动唤醒模型组：`gemini` 或 `claude_gpt`。 |
| `activation_models.antigravity.models` | 当前 Antigravity 模型组的模型名称。 |

说明：

- 仍可写带单位的 Go duration（如 `45m`、`2h`、`90s`）。
- 已废弃并忽略：`max_probe_interval`、`min_probe_interval`。

## 管理页面与接口

### 资源页面

- `GET /v0/resource/plugins/quota-activation/status`
  返回静态 HTML 页面，用于管理密钥验证、凭证选择与手动唤醒。页面内动态请求走 management API。

### 管理 API

以下接口需要 CPA 管理密钥：

- `GET /v0/management/quota-activation/status`
- `GET /v0/management/quota-activation/auth-files`
- `POST /v0/management/quota-activation/activate`
- `GET /v0/management/quota-activation/diagnostics`

模型列表使用宿主全局接口：

- `GET /v0/management/auth-files/models?name=...`

## 许可证

本项目使用 MIT License，详见 [LICENSE](./LICENSE)。
