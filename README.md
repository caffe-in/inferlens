# inferlens
A lightweight observability and benchmarking toolkit for self-hosted LLM inference services.

## v0.0.1 Goal
The current milestone is a local `ping` probe for vLLM-compatible inference services. It sends one streaming OpenAI-compatible chat request, captures a client-side request timeline, reads vLLM `/metrics` before and after the probe, and prints a readable performance diagnosis.

## Requirements
- Go 1.23+
- A local vLLM server exposing an OpenAI-compatible API on `http://localhost:8000`

## Quick Start
Start vLLM locally. Example:

```bash
vllm serve Qwen/Qwen2.5-0.5B-Instruct
```

Build the CLI:

```bash
mkdir -p artifacts
go build -o artifacts/inferlens ./cmd/inferlens
```

Run InferLens:

```bash
./artifacts/inferlens ping --model Qwen/Qwen2.5-0.5B-Instruct --prompt "hello"
```

Expected output:

```text
Hello! How can I help you today?

--- inferlens ping ---
endpoint: http://localhost:8000
model: Qwen/Qwen2.5-0.5B-Instruct
status: 200

client timeline:
  total: 742ms
  headers: 45ms
  first chunk: 210ms
  first token: 215ms
  stream: 532ms
  chunks: 12
  content deltas: 10
  output rate: 18.9 deltas/s

vllm metrics:
  request_success: +1
  prompt_tokens: +8
  generation_tokens: +10
  waiting: 0 -> 0
  running: 0 -> 0
  gpu_kv_cache: 12.0% -> 12.4%

diagnosis:
  first token latency looks normal
  no queue buildup observed in before/after snapshot
```

## Flags
- `--model`: model name served by vLLM
- `--prompt`: prompt text to send
- `--endpoint`: vLLM base URL, defaults to `http://localhost:8000`
- `--metrics-endpoint`: vLLM metrics URL, defaults to `<endpoint>/metrics`
- `--max-tokens`: maximum generated tokens for the probe, defaults to `128`
- `--timeout`: probe timeout, defaults to `60s`

## Notes
- v0.0.1 is intentionally a single active probe, not a benchmark loop.
- If `/metrics` is unavailable, InferLens still prints the client timeline and marks server metrics as unavailable.
- Local build artifacts should go under `artifacts/`, which is intentionally gitignored.
- Grafana, benchmarking, Kubernetes scheduling, and MLOps workflows are future milestones.
