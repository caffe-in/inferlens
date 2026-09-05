# inferlens
A lightweight observability and debugging CLI for self-hosted LLM inference services.

## v0.0.5 Goal
InferLens `ping` probes one inference request and prints a readable client timeline plus runtime-aware server observations.

`inferlens ping` is equivalent to `inferlens ping serve`. Serve mode supports native observation adapters for vLLM, llama.cpp, and SGLang. New in v0.0.5: `ping kserve` combines a read-only KServe control-plane snapshot, predictor Pod state, and one OpenAI-compatible streaming request against a user-provided endpoint. `ping api` remains runtime-agnostic and `ping offline` still runs local vLLM offline inference through the bundled Python helper.

## Requirements
- Go 1.23+
- For `ping serve`: a vLLM, llama.cpp, or SGLang OpenAI-compatible server
- For `ping kserve`: `kubectl` on PATH with read access to the target cluster
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
Use this for a local or self-hosted vLLM, llama.cpp, or SGLang server. It performs a best-effort `/health` check, reads Prometheus `/metrics` before and after one streaming chat completion, and maps runtime-specific metrics into common and native observations.

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

Probe SGLang: start the server with metrics enabled, then pass its endpoint explicitly:

```bash
python -m sglang.launch_server --model-path <model> --port 30000 --enable-metrics

./artifacts/inferlens ping serve \
  --runtime sglang \
  --endpoint http://localhost:30000 \
  --model <model> \
  --prompt "hello"
```

SGLang disables Prometheus metrics unless the server starts with `--enable-metrics`; without it, observations show as unavailable while the probe itself still succeeds.

Health and metrics collection are best-effort. If either is unavailable, the inference probe can still succeed and the report preserves the client timeline. See the [vLLM server interfaces](https://docs.vllm.ai/en/latest/serving/openai_compatible_server.html), [llama.cpp server interfaces](https://github.com/ggml-org/llama.cpp/blob/master/tools/server/README.md), and [SGLang production metrics](https://docs.sglang.ai/references/production_metrics) for runtime setup details.

### `ping api`
Use this for a user-provided OpenAI-compatible streaming API. It measures client-side streaming behavior only and does not select a runtime adapter or inspect server metrics.

```bash
OPENAI_API_KEY=... \
./artifacts/inferlens ping api \
  --endpoint https://api.example.com \
  --model your-model \
  --prompt "hello"
```

`OPENAI_API_KEY` is optional. When it is empty, InferLens sends no `Authorization` header. API mode requires streaming chat completions in v0.0.4.

### `ping offline`
Use this for one local vLLM offline inference. InferLens runs `scripts/vllm_offline_probe.py` internally and reports model load/generation timing.

```bash
./artifacts/inferlens ping offline \
  --python python3 \
  --model Qwen/Qwen2.5-0.5B-Instruct \
  --prompt "hello"
```

There is no `--helper` flag; the helper path is an internal packaging detail.

### `ping kserve`
Use this to diagnose an existing KServe `InferenceService` from your machine. One command combines a read-only control-plane snapshot, predictor Pod state, and one streaming chat request. InferLens stays local: it never mutates cluster resources, manages port-forwards, or collects multi-replica runtime metrics.

Terminal 1 — establish your own access path:

```bash
kubectl -n kserve-test port-forward svc/qwen-llama-predictor 8080:80
```

Terminal 2 — probe:

```bash
./artifacts/inferlens ping kserve \
  --name qwen-llama \
  --namespace kserve-test \
  --endpoint http://localhost:8080 \
  --model qwen2.5-1.5b-instruct \
  --prompt "hello"
```

- `--name` and `--endpoint` are required; `--namespace`, `--kubeconfig`, and `--context` default to kubectl's own precedence.
- The endpoint may include a route prefix, e.g. `http://127.0.0.1:8080/openai`.
- Kubernetes access goes through your local `kubectl`; only `get` operations are used. No `OPENAI_API_KEY` is read in this mode.
- Control-plane and data-plane results are reported separately. Exit is non-zero when the InferenceService cannot be read, its `Ready` condition is not `True`, predictor Pods cannot be listed, or the streaming request fails. Zero predictor Pods alone is not a failure — serverless services scale to zero.
- Direct engine-level metrics remain the job of `ping serve`; `ping kserve` prints ServingRuntime names and Pod images as facts without guessing the engine.

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
kserve:
  timeout: 60s
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
- `--runtime`: native observer for `serve`; `vllm` (default), `llamacpp`, or `sglang`
- `--endpoint`: OpenAI-compatible base URL for `serve`, `api`, or `kserve`
- `--name`: InferenceService name for `kserve`
- `--namespace`, `--kubeconfig`, `--context`: cluster selection for `kserve`, defaulting to kubectl's precedence
- `--metrics-endpoint`: Prometheus metrics URL for the selected `serve` runtime
- `--python`: Python interpreter for `offline`
- `--max-tokens`: maximum generated tokens, defaults to `128`
- `--timeout`: probe timeout; `offline` defaults to `0`, meaning no active timeout

## Notes
- v0.0.5 is still a single active probe, not a benchmark loop.
- `serve` ignores `OPENAI_API_KEY` so local probes do not inherit unrelated credentials.
- vLLM, llama.cpp, and SGLang are the native runtimes. Other OpenAI-compatible runtimes remain usable through `ping api` without server observations.
- `ping kserve` is the first consumer of the runtime-neutral probe layer against KServe-hosted deployments; it reads cluster state only and never writes.
- Runtime adapters are the data-plane foundation for a future KServe collector; v0.0.5 does not add in-cluster agents or multi-replica metric aggregation.
- Local build artifacts should go under `artifacts/`, which is intentionally gitignored.
- Grafana, benchmarking, Kubernetes scheduling, and MLOps workflows are future milestones.
