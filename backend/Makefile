# Cross-compile do agente de estacao (cmd/agent).
#
# CGO_ENABLED=0 gera binario estatico, sem dependencia de libc do destino —
# essencial para instalar em estacoes heterogeneas. -s -w tira simbolos e
# DWARF, reduzindo o tamanho do binario distribuido.

BIN_DIR := dist
LDFLAGS := -s -w

.PHONY: agent-all agent-linux-amd64 agent-linux-arm64 agent-windows-amd64 agent-darwin-arm64 clean

agent-all: agent-linux-amd64 agent-linux-arm64 agent-windows-amd64 agent-darwin-arm64

$(BIN_DIR):
	mkdir -p $(BIN_DIR)

agent-linux-amd64: | $(BIN_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/agent-linux-amd64 ./cmd/agent

agent-linux-arm64: | $(BIN_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/agent-linux-arm64 ./cmd/agent

agent-windows-amd64: | $(BIN_DIR)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/agent-windows-amd64.exe ./cmd/agent

agent-darwin-arm64: | $(BIN_DIR)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/agent-darwin-arm64 ./cmd/agent

clean:
	rm -rf $(BIN_DIR)
