DOCKER_BUILD_IMAGE = dippynark/kfmt-build:v1.0.0

BIN_DIR = bin
INPUT_DIR = input
OUTPUT_DIR = output
K8S_DIR = k8s.io
WORK_DIR = /workspace

generate:
	mkdir -p $(K8S_DIR)
	ls $(K8S_DIR)/api || git clone https://github.com/kubernetes/api $(K8S_DIR)/api
	ls $(K8S_DIR)/kube-aggregator || git clone https://github.com/kubernetes/kube-aggregator $(K8S_DIR)/kube-aggregator
	ls $(K8S_DIR)/apiextensions-apiserver || git clone https://github.com/kubernetes/apiextensions-apiserver $(K8S_DIR)/apiextensions-apiserver
	go run hack/discovery-gen.go -- $(K8S_DIR) discovery/local_discovery.go
	go fmt discovery/local_discovery.go

build:
	CGO_ENABLED=0 go build -o $(BIN_DIR)/kfmt -tags netgo

release:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $(BIN_DIR)/kfmt-linux-amd64 -tags netgo
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o $(BIN_DIR)/kfmt-darwin-amd64 -tags netgo
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o $(BIN_DIR)/kfmt-windows-amd64.exe -tags netgo
	sha256sum bin/kfmt-linux-amd64 bin/kfmt-darwin-amd64 bin/kfmt-windows-amd64.exe > $(BIN_DIR)/checksums.txt

test: go_test e2e_test

go_test:
	# https://github.com/golang/go/issues/28065#issuecomment-725632025
	CGO_ENABLED=0 go test -v

e2e_test:
	rm -rf $(OUTPUT_DIR)
	$(BIN_DIR)/kfmt --input $(INPUT_DIR) \
		--output $(OUTPUT_DIR) \
		--strict \
		--comment
	find $(OUTPUT_DIR)

docker_build_image:
	docker build \
		-t $(DOCKER_BUILD_IMAGE) \
		-f Dockerfile.build \
		.

docker_build_image_push: docker_build_image
	docker push $(DOCKER_BUILD_IMAGE)

docker_%: docker_build_image
	docker run -it \
		-w $(WORK_DIR) \
		-v $(GOPATH)/pkg/mod:/go/pkg/mod \
		-v $(CURDIR):$(WORK_DIR) \
		$(DOCKER_BUILD_IMAGE) \
		make $*
