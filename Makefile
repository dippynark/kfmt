KFMT = ./cmd/kfmt

BIN_DIR = bin
K8S_DIR = k8s.io

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

test:
	# https://github.com/golang/go/issues/28065#issuecomment-725632025
	CGO_ENABLED=0 go test -v ./...

build:
	CGO_ENABLED=0 go build -o $(BIN_DIR)/kfmt $(BUILD_FLAGS) $(KFMT)

install:
	CGO_ENABLED=0 go install $(BUILD_FLAGS) $(KFMT)

release:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $(BIN_DIR)/kfmt-linux-amd64 $(BUILD_FLAGS) $(KFMT)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o $(BIN_DIR)/kfmt-darwin-amd64 $(BUILD_FLAGS) $(KFMT)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o $(BIN_DIR)/kfmt-darwin-arm64 $(BUILD_FLAGS) $(KFMT)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o $(BIN_DIR)/kfmt-windows-amd64.exe $(BUILD_FLAGS) $(KFMT)
	cd $(BIN_DIR) && sha256sum kfmt-linux-amd64 kfmt-darwin-amd64 kfmt-darwin-arm64 kfmt-windows-amd64.exe > checksums.txt

docker_build_push:
	docker buildx build \
		-t $(DOCKER_BUILD_IMAGE) \
		--build-arg=VERSION=$(VERSION) \
		--platform linux/amd64,linux/arm64 \
		--push \
		.
