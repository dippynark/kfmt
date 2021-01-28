DOCKER_IMAGE = dippynark/kfmt-build

BIN_DIR = $(CURDIR)/bin

INPUT_DIR = $(CURDIR)/input
OUTPUT_DIR = $(CURDIR)/output

generate:
	ls api || git clone https://github.com/kubernetes/api
	go run hack/discovery-gen.go -- $(CURDIR)/api $(CURDIR)/discovery/local_discovery.go
	go fmt $(CURDIR)/discovery/local_discovery.go

build:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/kfmt .

test:
	rm -rf $(INPUT_DIR) $(OUTPUT_DIR)
	mkdir -p $(INPUT_DIR)
	# Download cert-manager manifests
	curl -L https://github.com/jetstack/cert-manager/releases/download/v1.1.0/cert-manager.yaml -o $(INPUT_DIR)/cert-manager.yaml
	$(BIN_DIR)/kfmt --input-dir $(INPUT_DIR) --remove-input \
		--output-dir $(OUTPUT_DIR) \
		--discovery \
		--comment
	rmdir $(INPUT_DIR)
	find $(OUTPUT_DIR)

docker_image:
	docker build \
		-t $(DOCKER_IMAGE) $(CURDIR)

docker_push: docker_image
	docker push $(DOCKER_IMAGE)

docker_%: docker_image
	docker run -it \
		-w /workspace \
		-v $(CURDIR):/workspace \
		$(DOCKER_IMAGE) \
		make $*
