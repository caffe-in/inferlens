# inferlens
A lightweight observability and debugging CLI for self-hosted LLM inference services.

## v0.0.3 Goal
InferLens `ping` probes one inference request and prints a readable client timeline plus runtime-aware server observations.

`inferlens ping` is equivalent to `inferlens ping serve`. Serve mode supports native observation adapters for vLLM and llama.cpp, while `ping api` remains runtime-agnostic for other OpenAI-compatible streaming APIs. `ping offline` continues to run local vLLM offline inference through the bundled Python helper.

## Requirements
- Go 1.23+
- For `ping serve`: a vLLM or llama.cpp OpenAI-compatible server
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
./artifacts/inferlens ping \
  --runtime vllm \
  --model Qwen/Qwen2.5-0.5B-Instruct \
  --prompt "hello"
```

This is the same as:

```bash
./artifacts/inferlens ping serve \
  --runtime vllm \
  --model Qwen/Qwen2.5-0.5B-Instruct \
  --prompt "hello"
```

## Ping Modes
### `ping serve`
Use this for a local or self-hosted vLLM or llama.cpp server. It performs a best-effort `/health` check, reads Prometheus `/metrics` before and after one streaming chat completion, and maps runtime-specific metrics into common and native observations.

Probe vLLM (the default runtime):

```bash
./artifacts/inferlens ping serve \
  --runtime vllm \
  --endpoint http://localhost:8000 \
  --model Qwen/Qwen2.5-0.5B-Instruct \
  --prompt "hello"
```

Start llama.cpp with its metrics endpoint enabled:

```bash
llama-server -m ./model.gguf --port 8080 --metrics
```

Then select its adapter and endpoint explicitly:

```bash
./artifacts/inferlens ping serve \
  --runtime llamacpp \
  --endpoint http://localhost:8080 \
  --model your-model \
  --prompt "hello"
```

InferLens does not auto-detect the runtime. The global endpoint default remains `http://localhost:8000`, so llama.cpp users normally pass `--endpoint http://localhost:8080`. llama.cpp returns `501` from `/metrics` unless `llama-server` starts with `--metrics`.

Health and metrics collection are best-effort. If either is unavailable, the inference probe can still succeed and the report preserves the client timeline. See the [vLLM server interfaces](https://docs.vllm.ai/en/latest/serving/openai_compatible_server.html) and [llama.cpp server interfaces](https://github.com/ggml-org/llama.cpp/blob/master/tools/server/README.md) for runtime setup details.

### `ping api`
Use this for a user-provided OpenAI-compatible streaming API. It measures client-side streaming behavior only and does not select a runtime adapter or inspect server metrics.

```bash
OPENAI_API_KEY=... \
./artifacts/inferlens ping api \
  --endpoint https://api.example.com \
  --model your-model \
  --prompt "hello"
```

`OPENAI_API_KEY` is optional. When it is empty, InferLens sends no `Authorization` header. API mode requires streaming chat completions in v0.0.3.

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
  runtime: vllm
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
- `--runtime`: native observer for `serve`; `vllm` (default) or `llamacpp`
- `--endpoint`: OpenAI-compatible base URL for `serve` or `api`
- `--metrics-endpoint`: Prometheus metrics URL for the selected `serve` runtime
- `--python`: Python interpreter for `offline`
- `--max-tokens`: maximum generated tokens, defaults to `128`
- `--timeout`: probe timeout; `offline` defaults to `0`, meaning no active timeout

## Notes
- v0.0.3 is still a single active probe, not a benchmark loop.
- `serve` ignores `OPENAI_API_KEY` so local probes do not inherit unrelated credentials.
- vLLM and llama.cpp are the native runtimes in v0.0.3. Other OpenAI-compatible runtimes remain usable through `ping api` without server observations.
- Runtime adapters are the data-plane foundation for a future KServe collector; v0.0.3 does not add Kubernetes or KServe behavior.
- Local build artifacts should go under `artifacts/`, which is intentionally gitignored.
- Grafana, benchmarking, Kubernetes scheduling, and MLOps workflows are future milestones.
