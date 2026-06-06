# inferlens
A lightweight observability and benchmarking toolkit for self-hosted LLM inference services.

## v0 Goal
The first milestone is a local CLI that sends one prompt to a locally running vLLM server and prints the response with basic request metrics.

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
./artifacts/inferlens chat --model Qwen/Qwen2.5-0.5B-Instruct --prompt "hello"
```

Expected output:

```text
Hello! How can I help you today?

status: 200
latency: 742ms
```

## Flags
- `--model`: model name served by vLLM
- `--prompt`: prompt text to send
- `--endpoint`: vLLM base URL, defaults to `http://localhost:8000`

## Notes
- v0 is intentionally minimal and synchronous.
- Current metrics only include HTTP status and total request latency.
- Local build artifacts should go under `artifacts/`, which is intentionally gitignored.
- Grafana, benchmarking, Kubernetes scheduling, and MLOps workflows are future milestones.
