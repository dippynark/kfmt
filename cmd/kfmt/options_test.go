package main

import (
	"bytes"
	"os"
	"path"
	"testing"

	"github.com/pkg/errors"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
)

const (
	outputDirectory = "output"
)

// requireDirectory checks that the path is a directory in the filesystem
func requireDirectory(fs afero.Fs, path string) error {
	info, err := fs.Stat(path)
	if err != nil {
		return errors.Errorf(err.Error())
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

// requireFileIsNotExist checks that the path does not exist
func requireFileIsNotExist(fs afero.Fs, path string) error {
	_, err := fs.Stat(path)
	if err == nil {
		return errors.Errorf("%s exists", path)
	}
	if !os.IsNotExist(err) {
		return errors.Errorf(err.Error())
	}
	return nil
}

func TestFilters(t *testing.T) {
	// Setup options
	o := &options{
		inputs:  []string{"input.yaml"},
		output:  outputDirectory,
		filters: []string{"Secret"},
	}

	// Setup memory backed filesystem
	fs := afero.NewMemMapFs()

	// Create input manifests
	manifests := `
apiVersion: v1
kind: Secret
metadata:
  name: test
`
	err := afero.WriteFile(fs, "input.yaml", []byte(manifests), 0644)
	require.Nil(t, err)

	// Format input manifests
	err = o.run(fs)
	require.Nil(t, err)

	// Ensure Secret has not been formatted
	err = requireFileIsNotExist(fs, path.Join(outputDirectory, "namespaces/default/secret-test.yaml"))
	require.Nil(t, err)

	// Remove filter and ensure Secret is formatted
	o.filters = []string{}
	err = o.run(fs)
	require.Nil(t, err)
	err = requireRegularFileContents(fs, path.Join(outputDirectory, "namespaces/default/secret-test.yaml"), `---
apiVersion: v1
kind: Secret
metadata:
  name: test
  namespace: default
`)
	require.Nil(t, err)
}

func TestGVKScopes(t *testing.T) {
	// Setup options
	o := &options{
		inputs:    []string{"input.yaml"},
		output:    outputDirectory,
		gvkScopes: []string{"Tester.test.io/v1:Namespaced"},
	}

	// Setup memory backed filesystem
	fs := afero.NewMemMapFs()

	// Create input manifests
	manifests := `
apiVersion: test.io/v1
kind: Tester
metadata:
  name: example
`
	err := afero.WriteFile(fs, "input.yaml", []byte(manifests), 0644)
	require.Nil(t, err)

	// Format input manifests
	err = o.run(fs)
	require.Nil(t, err)

	// Ensure tester resource is formatted
	err = requireRegularFileContents(fs, path.Join(outputDirectory, "namespaces/default/tester.test.io-example.yaml"), `---
apiVersion: test.io/v1
kind: Tester
metadata:
  name: example
  namespace: default
`)
	require.Nil(t, err)

	// Use cluster scope
	o.gvkScopes = []string{"Tester.test.io/v1:Cluster"}
	err = o.run(fs)
	require.Nil(t, err)
	err = requireRegularFileContents(fs, path.Join(outputDirectory, "cluster/testers.test.io/example.yaml"), `---
apiVersion: test.io/v1
kind: Tester
metadata:
  name: example
`)
	require.Nil(t, err)

	// Use unrecognised scope
	o.gvkScopes = []string{"Tester.test.io/v1:Foo"}
	err = o.run(fs)
	require.Equal(t, err.Error(), "unrecognised scope Foo")
}

func TestCRDScope(t *testing.T) {
	// Setup options
	o := &options{
		inputs: []string{"input.yaml"},
		output: outputDirectory,
	}

	// Setup memory backed filesystem
	fs := afero.NewMemMapFs()

	// Create input manifests
	manifests := `
apiVersion: test.io/v1
kind: Tester
metadata:
  name: example
`
	err := afero.WriteFile(fs, "input.yaml", []byte(manifests), 0644)
	require.Nil(t, err)

	// Check that formatting fails to determine scope
	err = o.run(fs)
	require.Equal(t, err.Error(), "failed to find Namespaces in input.yaml: could not find REST mapping for resource test.io/v1, Kind=Tester")

	// Create corresponding CRD and ensure resource is now formatted correctly
	crd := `
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
`
	err = afero.WriteFile(fs, "crd.yaml", []byte(crd), 0644)
	require.Nil(t, err)
	o.inputs = append(o.inputs, "crd.yaml")
	err = o.run(fs)
	require.Nil(t, err)
	err = requireRegularFileContents(fs, path.Join(outputDirectory, "namespaces/default/tester.test.io-example.yaml"), `---
apiVersion: test.io/v1
kind: Tester
metadata:
  name: example
  namespace: default
`)
	require.Nil(t, err)
}

func TestNamespace(t *testing.T) {
	// Setup options
	o := &options{
		inputs:    []string{"input.yaml"},
		output:    outputDirectory,
		namespace: "test",
	}

	// Setup memory backed filesystem
	fs := afero.NewMemMapFs()

	// Create input manifests
	manifests := `
apiVersion: v1
kind: Secret
metadata:
  name: test
`
	err := afero.WriteFile(fs, "input.yaml", []byte(manifests), 0644)
	require.Nil(t, err)

	// Format input manifests
	err = o.run(fs)
	require.Nil(t, err)

	// Ensure Secret is formatted
	err = requireRegularFileContents(fs, path.Join(outputDirectory, "namespaces/test/secret-test.yaml"), `---
apiVersion: v1
kind: Secret
metadata:
  name: test
  namespace: test
`)
	require.Nil(t, err)
}

func TestClean(t *testing.T) {
	// Setup options
	o := &options{
		inputs: []string{"input.yaml"},
		output: outputDirectory,
		clean:  true,
	}

	// Setup memory backed filesystem
	fs := afero.NewMemMapFs()

	// Create input manifests
	manifests := `
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: foo
  namespace: test
`
	err := afero.WriteFile(fs, "input.yaml", []byte(manifests), 0644)
	require.Nil(t, err)

	// Format input manifests
	err = o.run(fs)
	require.Nil(t, err)

	// Ensure resource is formatted
	err = requireRegularFileContents(fs, path.Join(outputDirectory, "cluster/clusterroles/foo.yaml"), `---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: foo
`)
	require.Nil(t, err)

	// Disable clean
	o.clean = false
	manifests = `
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: bar
  namespace: test
`
	err = afero.WriteFile(fs, "input.yaml", []byte(manifests), 0644)
	require.Nil(t, err)
	err = o.run(fs)
	require.Nil(t, err)
	err = requireRegularFileContents(fs, path.Join(outputDirectory, "cluster/clusterroles/bar.yaml"), `---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: bar
  namespace: test
`)
	require.Nil(t, err)
}

func TestStrict(t *testing.T) {
	// Setup options
	o := &options{
		inputs: []string{"input.yaml"},
		output: outputDirectory,
		strict: true,
	}

	// Setup memory backed filesystem
	fs := afero.NewMemMapFs()

	// Attempt to format ClusterRole with a namespace specified
	manifests := `
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: foo
  namespace: test
`
	err := afero.WriteFile(fs, "input.yaml", []byte(manifests), 0644)
	require.Nil(t, err)
	err = o.run(fs)
	require.Equal(t, err.Error(), "metadata.namespace field should not be set for cluster-scoped resource: rbac.authorization.k8s.io/v1, Kind=ClusterRole")

	// Disable strict
	o.strict = false
	err = o.run(fs)
	require.Nil(t, err)
	err = requireRegularFileContents(fs, path.Join(outputDirectory, "cluster/clusterroles/foo.yaml"), `---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: foo
  namespace: test
`)
	require.Nil(t, err)
}

func TestRemove(t *testing.T) {
	// Setup options
	o := &options{
		inputs: []string{"input.yaml"},
		output: outputDirectory,
		remove: false,
	}

	// Setup memory backed filesystem
	fs := afero.NewMemMapFs()

	// Create input manifests
	manifests := `
apiVersion: v1
kind: Secret
metadata:
  name: test
`
	err := afero.WriteFile(fs, "input.yaml", []byte(manifests), 0644)
	require.Nil(t, err)
	err = o.run(fs)
	require.Nil(t, err)
	err = requireRegularFileContents(fs, "input.yaml", `
apiVersion: v1
kind: Secret
metadata:
  name: test
`)
	require.Nil(t, err)

	// Remove formatted resource
	err = fs.Remove(path.Join(outputDirectory, "namespaces/default/secret-test.yaml"))
	require.Nil(t, err)

	// Enable remove and verify input file is removed
	o.remove = true
	err = o.run(fs)
	require.Nil(t, err)
	err = requireFileIsNotExist(fs, "input.yaml")
	require.Nil(t, err)
}

func TestComment(t *testing.T) {
	// Setup options
	o := &options{
		inputs:  []string{"input.yaml"},
		output:  outputDirectory,
		comment: true,
	}

	// Setup memory backed filesystem
	fs := afero.NewMemMapFs()

	// Create input manifests
	manifests := `
apiVersion: v1
kind: Secret
metadata:
  name: test
`
	err := afero.WriteFile(fs, "input.yaml", []byte(manifests), 0644)
	require.Nil(t, err)
	err = o.run(fs)
	require.Nil(t, err)
	err = requireRegularFileContents(fs, path.Join(outputDirectory, "namespaces/default/secret-test.yaml"), `---
# Source: input.yaml
apiVersion: v1
kind: Secret
metadata:
  name: test
  namespace: default
`)
	require.Nil(t, err)
}

func TestOverwrite(t *testing.T) {
	// Setup options
	o := &options{
		inputs: []string{"input.yaml"},
		output: outputDirectory,
	}

	// Setup memory backed filesystem
	fs := afero.NewMemMapFs()

	// Create input manifests
	manifests := `
apiVersion: v1
kind: Secret
metadata:
  name: test
`
	err := afero.WriteFile(fs, "input.yaml", []byte(manifests), 0644)
	require.Nil(t, err)
	err = o.run(fs)
	require.Nil(t, err)
	err = o.run(fs)
	require.Equal(t, err.Error(), "file already exists: output/namespaces/default/secret-test.yaml")

	// Set overwrite
	o.overwrite = true
	err = o.run(fs)
	require.Nil(t, err)
}

func TestCreateMissingNamespaces(t *testing.T) {
	// Setup options
	o := &options{
		inputs:                  []string{"input.yaml"},
		output:                  outputDirectory,
		createMissingNamespaces: true,
	}

	// Setup memory backed filesystem
	fs := afero.NewMemMapFs()

	// Create input manifests
	manifests := `
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
	err := afero.WriteFile(fs, "input.yaml", []byte(manifests), 0644)
	require.Nil(t, err)

	// Format input manifests
	err = o.run(fs)
	require.Nil(t, err)

	// Ensure nginx namespace is created
	err = requireRegularFileContents(fs, path.Join(outputDirectory, "cluster/namespaces/nginx.yaml"), `---
apiVersion: v1
kind: Namespace
metadata:
  name: nginx
`)
	require.Nil(t, err)
}

func TestVersion(t *testing.T) {
	// Setup options
	o := &options{
		inputs:  []string{"input.yaml"},
		output:  outputDirectory,
		version: true,
	}

	// Setup memory backed filesystem
	fs := afero.NewMemMapFs()

	// Create input manifests
	manifests := `
apiVersion: v1
kind: Secret
metadata:
  name: test
`
	err := afero.WriteFile(fs, "input.yaml", []byte(manifests), 0644)
	require.Nil(t, err)
	err = o.run(fs)
	require.Nil(t, err)
	err = requireFileIsNotExist(fs, path.Join(outputDirectory, "namespaces/default/secret-test.yaml"))
	require.Nil(t, err)

	// Disable version
	o.version = false
	err = o.run(fs)
	require.Nil(t, err)
	err = requireRegularFileContents(fs, path.Join(outputDirectory, "namespaces/default/secret-test.yaml"), `---
apiVersion: v1
kind: Secret
metadata:
  name: test
  namespace: default
`)
	require.Nil(t, err)
}

func TestNamespacesAnnotation(t *testing.T) {
	// Setup options
	o := &options{
		inputs: []string{"input.yaml"},
		output: outputDirectory,
	}

	// Setup memory backed filesystem
	fs := afero.NewMemMapFs()

	// Create input manifests
	manifests := `
apiVersion: v1
kind: ResourceQuota
metadata:
  name: pods-high
  namespace: test
  annotations:
    kfmt.dev/namespaces: "*,-bar"
    another: annotation
spec:
  hard:
    pods: "20"
---
apiVersion: v1
kind: Namespace
metadata:
  name: foo
---
apiVersion: v1
kind: Namespace
metadata:
  name: bar
`
	err := afero.WriteFile(fs, "input.yaml", []byte(manifests), 0644)
	require.Nil(t, err)
	err = o.run(fs)
	require.Nil(t, err)

	// Require ResourceQuota exists in the foo Namespace
	err = requireRegularFileContents(fs, path.Join(outputDirectory, "namespaces/foo/resourcequota-pods-high.yaml"), `---
apiVersion: v1
kind: ResourceQuota
metadata:
  name: pods-high
  namespace: foo
  annotations:
    another: annotation
spec:
  hard:
    pods: "20"
`)
	require.Nil(t, err)

	// Require ResourceQuota is missing in the bar Namespace
	err = requireFileIsNotExist(fs, path.Join(outputDirectory, "namespaces/bar/resourcequota-pods-high.yaml"))
	require.Nil(t, err)

	// Require ResourceQuota is missing in the default Namespace
	err = requireFileIsNotExist(fs, path.Join(outputDirectory, "namespaces/default/resourcequota-pods-high.yaml"))
	require.Nil(t, err)

	// Verify that specifying a missing Namespace causes an error
	manifests = `
apiVersion: v1
kind: ResourceQuota
metadata:
  name: pods-high
  annotations:
    kfmt.dev/namespaces: example
spec:
  hard:
    pods: "20"
`
	err = afero.WriteFile(fs, "input.yaml", []byte(manifests), 0644)
	require.Nil(t, err)
	err = o.run(fs)
	require.Equal(t, err.Error(), "Namespace \"example\" not found when processing annotation kfmt.dev/namespaces")
}

// TODO: Test discovery and kubeconfig
