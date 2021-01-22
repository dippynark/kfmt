BIN_DIR = $(CURDIR)/bin

generate:
	go run hack/discovery-gen.go -- $(CURDIR)/discovery/local_discovery.go
	go fmt $(CURDIR)/discovery/local_discovery.go

build:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/kfmt .

run:
	rm -rf output
	$(BIN_DIR)/kfmt --input-dir input --output-dir output --discovery
