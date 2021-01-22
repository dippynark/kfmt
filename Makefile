BIN_DIR = $(CURDIR)/bin

INPUT_DIR = $(CURDIR)/input
OUTPUT_DIR = $(CURDIR)/output

generate:
	go run hack/discovery-gen.go -- $(CURDIR)/discovery/local_discovery.go
	go fmt $(CURDIR)/discovery/local_discovery.go

build:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/kfmt .

test:
	rm -rf $(INPUT_DIR) $(OUTPUT_DIR)
	mkdir -p $(INPUT_DIR)
	# Download cert-manager manifests
	curl -L https://github.com/jetstack/cert-manager/releases/download/v1.1.0/cert-manager.yaml -o $(INPUT_DIR)/cert-manager.yaml
	$(BIN_DIR)/kfmt --input-dir $(INPUT_DIR) --output-dir $(OUTPUT_DIR) --discovery
