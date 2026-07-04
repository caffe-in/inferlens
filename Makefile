BIN := artifacts/inferlens
MODEL ?= Qwen/Qwen2.5-0.5B-Instruct
PROMPT ?= hello
ENDPOINT ?= http://localhost:8000
MAX_TOKENS ?= 128
TIMEOUT ?= 60s

.PHONY: build test ping ping-api ping-offline clean help

build:
	mkdir -p artifacts
	go build -o $(BIN) ./cmd/inferlens

test:
	go test ./...

ping: build
	$(BIN) ping serve \
		--endpoint $(ENDPOINT) \
		--model $(MODEL) \
		--prompt "$(PROMPT)" \
		--max-tokens $(MAX_TOKENS) \
		--timeout $(TIMEOUT)

ping-api: build
	$(BIN) ping api \
		--endpoint $(ENDPOINT) \
		--model $(MODEL) \
		--prompt "$(PROMPT)" \
		--max-tokens $(MAX_TOKENS) \
		--timeout $(TIMEOUT)

ping-offline: build
	$(BIN) ping offline \
		--model $(MODEL) \
		--prompt "$(PROMPT)" \
		--max-tokens $(MAX_TOKENS)

clean:
	rm -f $(BIN)

help:
	@echo "Targets:"
	@echo "  make build    Build $(BIN)"
	@echo "  make test     Run Go tests"
	@echo "  make ping     Build and run a local vLLM serve probe"
	@echo "  make ping-api Build and run a streaming API probe"
	@echo "  make ping-offline Build and run a local vLLM offline probe"
	@echo "  make clean    Remove build artifact"
	@echo ""
	@echo "Overrides:"
	@echo "  MODEL=<model> PROMPT=<text> ENDPOINT=<url> MAX_TOKENS=<n> TIMEOUT=<duration>"
