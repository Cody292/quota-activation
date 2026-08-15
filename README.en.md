<div align="center">

# Quota-Activation

[中文](./README.md) | [English](./README.en.md)

</div>

Quota Activation is a CLIProxyAPI (CPA) plugin for quota-reset activation. The plugin ID, dynamic library basename, and CPA configuration key are all `quota-activation`.

## Navigation

- [Overview](#overview)
- [Workflow](#workflow)
- [Build and Installation](#build-and-installation)
- [Plugin Store Source](#plugin-store-source)
- [Configuration](#configuration)
- [Management Page and API](#management-page-and-api)
- [License](#license)

## Overview

- Supports manual and automatic activation for **Codex** and **Antigravity** credentials.
- Keeps automatic and manual configuration separate. Automatic mode only uses `auto_activate`, `scan_interval`, and `activation_models.*`.
- Automatic quota windows prioritize real long-window quota data.
- Codex ignores 5-hour rolling short windows. When a real long window is absent, it synthesizes one from the plan type (paid 7 days, free or unknown 30 days).
- Antigravity keeps its 7-day default long window.
- Management pages and diagnostics expose only redacted information.

## Workflow

```text
Load plugin
  -> Read plugins.configs.quota-activation
  -> Register management routes and static resource page /status
  -> Manual: user selects credential/model and POSTs /activate
  -> Automatic: when auto_activate=true, scan by scan_interval
       - list/get_runtime credentials
       - prefer real long-window quota data
       - Codex ignores 5-hour rolling short windows; synthesize a long window from the plan (paid 7 days, free/unknown 30 days) when absent
       - Antigravity keeps the 7-day default long window
       - skip cycles already marked success (5-hour short windows never hard-skip)
       - share the same activation core with the manual path
       - default direct_http (host.http.do); optional scheduler_boost
```

## Build and Installation

The plugin runs as a CGO dynamic library. CPA derives the plugin ID from the filename, so the artifact must stay `quota-activation.<ext>`.

```bash
CGO_ENABLED=1 go build -buildmode=c-shared -o quota-activation.so .
```

Or:

```bash
./scripts/build.sh
```

Place the artifact in one of:

- `plugins/<GOOS>/<GOARCH>/quota-activation.<ext>`
- `plugins/<GOOS>/<GOARCH>-<variant>/quota-activation.<ext>`
- `plugins/quota-activation.<ext>`

Extensions: `.so` on Linux/FreeBSD, `.dylib` on macOS, `.dll` on Windows.

## Plugin Store Source

Third-party store sources must point to the raw JSON text of `registry.json`:

```yaml
plugins:
  enabled: true
  store-sources:
    - "https://raw.githubusercontent.com/Cody292/quota-activation/main/registry.json"
```

Do not use a GitHub HTML `blob` URL. After changing `store-sources`, restart CPA or reload configuration, then refresh the plugin store list.

## Configuration

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
      # default direct_http: host.http.do to upstream; no priority write / scheduler path
      activation_transport: "direct_http"
      # fall back to legacy scheduler_boost only on transport/host failures; never on business failures
      scheduler_boost_fallback: true
      activation_models:
        codex:
          models: "gpt-5.4-mini"
        antigravity:
          models_group: "gemini"
          models: "gemini-3-flash"
```

| Field | Description |
| :--- | :--- |
| `enabled` | Plugin switch. Also requires `plugins.enabled: true`. |
| `auto_activate` | Enable automatic activation. Default `false`. |
| `enable_before_activation` | When true, enable disabled credentials before activation and keep them enabled. |
| `scan_interval` | Auto-scan interval in **minutes**. Plain number, no unit required. Default `30`. |
| `activation_request_timeout` | Activation timeout in **seconds**. Plain number. Default `60`. |
| `max_concurrency` | Concurrency limit for automatic scan activation (worker pool size). Default `1`. |
| `activation_prompt` | Activation prompt (`direct_http` body; `scheduler_boost` via `host.model.execute`). |
| `activation_transport` | Transport: `direct_http` (**default**, via `host.http.do`) or `scheduler_boost` (legacy priority boost + `host.model.execute`). |
| `scheduler_boost_fallback` | Only when `activation_transport=direct_http`. Default `true`. On **transport/host** failures, may fall back once to `scheduler_boost`. **Business failures / fake 2xx (no valid structure) never fall back** and must not write a success cycle. |
| `activation_models.codex.models` | Codex model for automatic activation. |
| `activation_models.antigravity.models_group` | Antigravity group: `gemini` or `claude_gpt`. |
| `activation_models.antigravity.models` | Model for the selected Antigravity group. |

Notes:

- Unit-suffixed Go durations such as `45m`, `2h`, and `90s` are still accepted.
- Auto scan and manual activation share `Activator.Activate`. The `scan_interval` gate allows ~2s early ticks so timer jitter does not drop a round.
- Management UI, diagnostics, and run history surface **Simplified Chinese** failure text (legacy English strings are mapped).

## Management Page and API

### Resource page

- `GET /v0/resource/plugins/quota-activation/status`
  Static HTML page for key verification and manual activation. Dynamic calls use management APIs.

### Management API

Requires the CPA management key:

- `GET /v0/management/quota-activation/status`
- `GET /v0/management/quota-activation/auth-files`
- `POST /v0/management/quota-activation/activate`
- `GET /v0/management/quota-activation/diagnostics`

Model list uses the host global route:

- `GET /v0/management/auth-files/models?name=...`

## License

MIT License. See [LICENSE](./LICENSE).
