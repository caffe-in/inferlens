BIN := artifacts/inferlens
MODEL ?= Qwen/Qwen2.5-0.5B-Instruct
PROMPT ?= hello
ENDPOINT ?= http://localhost:8000
MAX_TOKENS ?= 128
TIMEOUT ?= 60s

.PHONY: build test ping clean help

build:
	mkdir -p artifacts
	go build -o $(BIN) ./cmd/inferlens

test:
	go test ./...

ping: build
	$(BIN) ping \
		--endpoint $(ENDPOINT) \
		--model $(MODEL) \
		--prompt "$(PROMPT)" \
		--max-tokens $(MAX_TOKENS) \
		--timeout $(TIMEOUT)

clean:
	rm -f $(BIN)

help:
	@echo "Targets:"
	@echo "  make build    Build $(BIN)"
	@echo "  make test     Run Go tests"
	@echo "  make ping     Build and run a local vLLM probe"
	@echo "  make clean    Remove build artifact"
	@echo ""
	@echo "Overrides:"
	@echo "  MODEL=<model> PROMPT=<text> ENDPOINT=<url> MAX_TOKENS=<n> TIMEOUT=<duration>"
