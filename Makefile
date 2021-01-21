run:
	rm -rf output
	go run ./main.go --input-dir input --output-dir output --kubeconfig /Users/luke/.kube/config --discovery
