# inferlens
A lightweight observability and debugging CLI for self-hosted LLM inference services.

## v0.0.2 Goal
InferLens `ping` probes one inference request and prints a readable client timeline plus mode-specific diagnostics.

`inferlens ping` is equivalent to `inferlens ping serve` and remains focused on the local vLLM serve loop. v0.0.2 also adds `ping api` for user-provided OpenAI-compatible streaming APIs and `ping offline` for local vLLM offline inference through the bundled Python helper.

## Requirements
- Go 1.23+
- For `ping serve`: a vLLM OpenAI-compatible server, usually `http://localhost:8000`
- For `ping offline`: a Python environment with `vllm` installed

## Quick Start
Start vLLM locally:

```bash
vllm serve Qwen/Qwen2.5-0.5B-Instruct
```

Build the CLI:

```bash
make build
```

Run the default serve probe:

```bash
./artifacts/inferlens ping --model Qwen/Qwen2.5-0.5B-Instruct --prompt "hello"
```

This is the same as:

```bash
./artifacts/inferlens ping serve --model Qwen/Qwen2.5-0.5B-Instruct --prompt "hello"
```

## Ping Modes
### `ping serve`
Use this for a local or self-hosted vLLM server. It sends one streaming chat completion request and reads vLLM `/metrics` before and after the probe.

```bash
./artifacts/inferlens ping serve \
  --endpoint http://localhost:8000 \
  --model Qwen/Qwen2.5-0.5B-Instruct \
  --prompt "hello"
```

If `/metrics` is unavailable, the probe can still succeed and the report marks server metrics as unavailable.

### `ping api`
Use this for a user-provided OpenAI-compatible streaming API. It measures client-side streaming behavior only and does not inspect vLLM server metrics.

```bash
OPENAI_API_KEY=... \
./artifacts/inferlens ping api \
  --endpoint https://api.example.com \
  --model your-model \
  --prompt "hello"
```

`OPENAI_API_KEY` is optional. When it is empty, InferLens sends no `Authorization` header. API mode requires streaming chat completions in v0.0.2.

### `ping offline`
Use this for one local vLLM offline inference. InferLens runs `scripts/vllm_offline_probe.py` internally and reports model load/generation timing.

```bash
./artifacts/inferlens ping offline \
  --python python3 \
  --model Qwen/Qwen2.5-0.5B-Instruct \
  --prompt "hello"
```

There is no `--helper` flag; the helper path is an internal packaging detail.

## Configuration
InferLens reads ping defaults in this order, with later layers overriding earlier layers:

```text
Go built-in defaults < cfg/default.yaml < --config < environment < CLI flags
```

Default config shape:

```yaml
serve:
  endpoint: http://localhost:8000
  timeout: 60s
api:
  timeout: 60s
offline:
  python: python3
  timeout: 0
```

Use a custom config file with:

```bash
./artifacts/inferlens ping --config ./my-inferlens.yaml api --model your-model --prompt "hello"
```

Environment variables:

- `OPENAI_BASE_URL`: default endpoint for `ping api`
- `OPENAI_API_KEY`: bearer token for `ping api`
- `INFERLENS_PYTHON`: default Python interpreter for `ping offline`

API tokens are not read from config files or CLI flags.

## Flags
- `--config`: ping YAML config path
- `--model`: model name
- `--prompt`: prompt text
- `--endpoint`: OpenAI-compatible base URL for `serve` or `api`
- `--metrics-endpoint`: vLLM metrics URL for `serve`
- `--python`: Python interpreter for `offline`
- `--max-tokens`: maximum generated tokens, defaults to `128`
- `--timeout`: probe timeout; `offline` defaults to `0`, meaning no active timeout

## Notes
- v0.0.2 is still a single active probe, not a benchmark loop.
- `serve` ignores `OPENAI_API_KEY` so local probes do not inherit unrelated credentials.
- Local build artifacts should go under `artifacts/`, which is intentionally gitignored.
- Grafana, benchmarking, Kubernetes scheduling, and MLOps workflows are future milestones.
