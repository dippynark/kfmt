generate:
	go run hack/discovery-gen.go -- $(CURDIR)/discovery/local_discovery.go
	go fmt $(CURDIR)/discovery/local_discovery.go

run:
	rm -rf output
	go run ./main.go --input-dir input --output-dir output --discovery
