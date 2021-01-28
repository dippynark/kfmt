DOCKER_BUILD_IMAGE = dippynark/kfmt-build:v1.0.0

BIN_DIR = $(CURDIR)/bin

INPUT_DIR = $(CURDIR)/input
OUTPUT_DIR = $(CURDIR)/output

WORK_DIR = /workspace

generate:
	ls api || git clone https://github.com/kubernetes/api
	go run hack/discovery-gen.go -- $(CURDIR)/api $(CURDIR)/discovery/local_discovery.go
	go fmt $(CURDIR)/discovery/local_discovery.go

UNAME_S := $(shell uname -s)
build:
	mkdir -p $(BIN_DIR)
ifeq ($(UNAME_S),Linux)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $(BIN_DIR)/kfmt -a -tags netgo .
else
	go build -o $(BIN_DIR)/kfmt .
endif

test:
	rm -rf $(INPUT_DIR) $(OUTPUT_DIR)
	mkdir -p $(INPUT_DIR)
	# Download cert-manager manifests
	curl -L https://github.com/jetstack/cert-manager/releases/download/v1.1.0/cert-manager.yaml -o $(INPUT_DIR)/cert-manager.yaml
	$(BIN_DIR)/kfmt --input-dir $(INPUT_DIR) --remove-input \
		--output-dir $(OUTPUT_DIR) \
		--comment
	rmdir $(INPUT_DIR)
	find $(OUTPUT_DIR)

docker_build_image:
	docker build \
		-t $(DOCKER_BUILD_IMAGE) \
		-f Dockerfile.build \
		$(CURDIR)

docker_build_image_push: docker_build_image
	docker push $(DOCKER_BUILD_IMAGE)

docker_%: docker_build_image
	docker run -it \
		-w $(WORK_DIR) \
		-v $(GOPATH)/pkg/mod:/go/pkg/mod \
		-v $(CURDIR):$(WORK_DIR) \
		$(DOCKER_BUILD_IMAGE) \
		make $*
