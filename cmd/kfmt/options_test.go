package main

import (
	"bytes"
	"os"
	"testing"

	"github.com/pkg/errors"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
)

// requireDirectory checks that the path is a directory in the filesystem
func requireDirectory(fs afero.Fs, path string) error {
	info, err := fs.Stat(path)
	if err != nil {
		return errors.Errorf(err.Error())
	}
	if os.IsNotExist(err) {
		return err
	}
	if !info.IsDir() {
		return errors.Errorf("%s is not a directory", path)
	}
	return nil
}

// requireRegularFileContents checks that the path is a regular file in the filesystem with the provided contents
func requireRegularFileContents(fs afero.Fs, path string, contents string) error {
	info, err := fs.Stat(path)
	if err != nil {
		return errors.Errorf(err.Error())
	}
	if os.IsNotExist(err) {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.Errorf("%s is not a regular file", path)
	}
	fileBytes, err := afero.ReadFile(fs, path)
	if err != nil {
		return err
	}
	expectedBytes := []byte(contents)
	if bytes.Compare(fileBytes, expectedBytes) != 0 {
		return errors.Errorf("unexpected file contents:\n%s", fileBytes)
	}
	return nil
}

func TestBasic(t *testing.T) {
	// Setup memory backed filesystems
	input := afero.NewMemMapFs()
	output := afero.NewMemMapFs()

	// Create input manifests
	manifests := `
apiVersion: v1
kind: Namespace
metadata:
  name: test
---
apiVersion: v1
kind: Secret
metadata:
  name: test
`
	err := afero.WriteFile(input, "input.yaml", []byte(manifests), 0644)
	require.Nil(t, err)

	// Format input manifests
	o := &options{inputs: []string{"input.yaml"}}
	err = o.format(input, output)
	require.Nil(t, err)

	// Ensure expected directories exist
	err = requireDirectory(output, "cluster")
	require.Nil(t, err)
	err = requireDirectory(output, "cluster/namespaces")
	require.Nil(t, err)
	err = requireDirectory(output, "namespaces")
	require.Nil(t, err)

	// Ensure namespace file exists and has expected contents
	err = requireRegularFileContents(output, "cluster/namespaces/test.yaml", `---
apiVersion: v1
kind: Namespace
metadata:
  name: test
`)
	require.Nil(t, err)

	// Ensure secret file exists and has expected contents
	err = requireRegularFileContents(output, "namespaces/default/secret-test.yaml", `---
apiVersion: v1
kind: Secret
metadata:
  name: test
  namespace: default
`)
	require.Nil(t, err)
}

func TestNamespaceCreation(t *testing.T) {
	// Setup memory backed filesystems
	input := afero.NewMemMapFs()
	output := afero.NewMemMapFs()

	// Create input manifests
	manifests := `
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx
  namespace: nginx
spec:
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - name: nginx
        image: nginx
        ports:
        - containerPort: 80
`
	err := afero.WriteFile(input, "input.yaml", []byte(manifests), 0644)
	require.Nil(t, err)

	// Format input manifests
	o := &options{
		inputs:                  []string{"input.yaml"},
		createMissingNamespaces: true,
	}
	err = o.format(input, output)
	require.Nil(t, err)

	// Ensure nginx namespace exists
	err = requireRegularFileContents(output, "cluster/namespaces/nginx.yaml", `---
apiVersion: v1
kind: Namespace
metadata:
  name: nginx
`)
	require.Nil(t, err)

	// Ensure nginx deployment exists
	err = requireRegularFileContents(output, "namespaces/nginx/deployment-nginx.yaml", `---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx
  namespace: nginx
spec:
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
        - name: nginx
          image: nginx
          ports:
            - containerPort: 80
`)
	require.Nil(t, err)
}

func TestCustomResourceDefinition(t *testing.T) {
	// Setup memory backed filesystems
	input := afero.NewMemMapFs()
	output := afero.NewMemMapFs()

	// Create input manifests
	manifests := `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: testers.test.io
spec:
  group: test.io
  names:
    kind: Tester
  scope: Namespaced
  versions:
  - name: v1
  - name: v1beta1
`
	err := afero.WriteFile(input, "input.yaml", []byte(manifests), 0644)
	require.Nil(t, err)

	// Format input manifests
	o := &options{inputs: []string{"input.yaml"}}
	err = o.format(input, output)
	require.Nil(t, err)

	// Ensure namespace file exists and has expected contents
	err = requireRegularFileContents(output, "cluster/customresourcedefinitions/testers.test.io.yaml", `---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: testers.test.io
spec:
  group: test.io
  names:
    kind: Tester
  scope: Namespaced
  versions:
    - name: v1
    - name: v1beta1
`)
	require.Nil(t, err)
}

func TestDefaultResourceQuota(t *testing.T) {
	// Setup memory backed filesystems
	input := afero.NewMemMapFs()
	output := afero.NewMemMapFs()

	// Create input manifests
	manifests := `
apiVersion: v1
kind: Namespace
metadata:
  name: example
---
apiVersion: v1
kind: ResourceQuota
metadata:
  name: pods-high
  annotations:
    kfmt.dev/namespaces: "*,-test"
spec:
  hard:
    pods: "20"
`
	err := afero.WriteFile(input, "input.yaml", []byte(manifests), 0644)
	require.Nil(t, err)

	// Format input manifests
	o := &options{inputs: []string{"input.yaml"}}
	err = o.format(input, output)
	require.Nil(t, err)

	// Ensure namespace file exists and has expected contents
	err = requireRegularFileContents(output, "namespaces/example/resourcequota-pods-high.yaml", `---
apiVersion: v1
kind: ResourceQuota
metadata:
  name: pods-high
  namespace: example
spec:
  hard:
    pods: "20"
`)
	require.Nil(t, err)
}
