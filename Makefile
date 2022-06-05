BIN_DIR = bin
INPUT_DIR = input
OUTPUT_DIR = output
K8S_DIR = k8s.io
WORK_DIR = /workspace
GOPATH ?= $(HOME)/go

VERSION = $(shell git describe --tags)
BUILD_FLAGS = -tags netgo -ldflags "-X main.version=$(VERSION)"
DOCKER_BUILD_IMAGE = dippynark/kfmt:$(VERSION)

generate:
	mkdir -p $(K8S_DIR)
	ls $(K8S_DIR)/api || git clone https://github.com/kubernetes/api $(K8S_DIR)/api
	ls $(K8S_DIR)/kube-aggregator || git clone https://github.com/kubernetes/kube-aggregator $(K8S_DIR)/kube-aggregator
	ls $(K8S_DIR)/apiextensions-apiserver || git clone https://github.com/kubernetes/apiextensions-apiserver $(K8S_DIR)/apiextensions-apiserver
	go run hack/discovery-gen.go -- $(K8S_DIR) pkg/discovery/local_discovery.go
	go fmt pkg/discovery/local_discovery.go

build:
	CGO_ENABLED=0 go build -o $(BIN_DIR)/kfmt $(BUILD_FLAGS) ./cmd/kfmt

release:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $(BIN_DIR)/kfmt-linux-amd64 $(BUILD_FLAGS) ./cmd/kfmt
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o $(BIN_DIR)/kfmt-darwin-amd64 $(BUILD_FLAGS) ./cmd/kfmt
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o $(BIN_DIR)/kfmt-windows-amd64.exe $(BUILD_FLAGS) ./cmd/kfmt
	cd $(BIN_DIR) && sha256sum kfmt-linux-amd64 kfmt-darwin-amd64 kfmt-windows-amd64.exe > checksums.txt

test:
	# https://github.com/golang/go/issues/28065#issuecomment-725632025
	CGO_ENABLED=0 go test -v ./...

e2e_test:
	rm -rf $(OUTPUT_DIR)
	$(BIN_DIR)/kfmt --input $(INPUT_DIR) \
		--output $(OUTPUT_DIR) \
		--strict \
		--comment \
		--create-missing-namespaces
	find $(OUTPUT_DIR)

docker_build:
	docker build \
		-t $(DOCKER_BUILD_IMAGE) \
		--build-arg=VERSION=$(VERSION) \
		.

docker_push: docker_build
	docker push $(DOCKER_BUILD_IMAGE)
