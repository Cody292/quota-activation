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
- Prefers real `quota_payload` windows, then infers from successful state history, then falls back to long provider windows.
- Management pages and diagnostics expose only redacted information.

## Workflow

```text
Load plugin
  -> Read plugins.configs.quota-activation
  -> Register management routes and static resource page /status
  -> Manual: user selects credential/model and POSTs /activate
  -> Automatic: when auto_activate=true, scan by scan_interval
       - list/get_runtime credentials
       - real quota payload first, then state inference, then fallback
       - skip cycles already marked success
       - send activation via host.model.execute
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
      activation_models:
        codex:
          models: "gpt-5-mini"
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
| `max_concurrency` | Max concurrent activations. Expected value is `1`. |
| `activation_prompt` | Prompt sent through `host.model.execute`. |
| `activation_models.codex.models` | Codex model for automatic activation. |
| `activation_models.antigravity.models_group` | Antigravity group: `gemini` or `claude_gpt`. |
| `activation_models.antigravity.models` | Model for the selected Antigravity group. |

Notes:

- Unit-suffixed Go durations such as `45m`, `2h`, and `90s` are still accepted.

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
