package main

import (
	"os"
	"reflect"
	"testing"

	"github.com/dippynark/kfmt/discovery"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

func TestFindNamespaces(t *testing.T) {
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
	expectedNamespaces := map[string]struct{}{"test": {}, "default": {}}

	o := Options{}

	resourceInspector, err := discovery.NewLocalResourceInspector()
	if err != nil {
		t.Error(err)
	}

	nodes, err := kio.FromBytes([]byte(manifests))
	if err != nil {
		t.Error(err)
	}

	namespaces, err := o.findNamespaces(nodes, resourceInspector)
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(namespaces, expectedNamespaces) {
		t.Error("failed to find expected namespaces")
	}
}

func TestFilterNodes(t *testing.T) {
	manifests := `
apiVersion: v1
kind: Secret
metadata:
  name: test
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: test
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test
`

	o := Options{filters: []string{"Deployment.apps", "Secret"}}

	nodes, err := kio.FromBytes([]byte(manifests))
	if err != nil {
		t.Error(err)
	}
	yamlFileNodes := map[string][]*yaml.RNode{os.Stdin.Name(): nodes}

	err = o.filterNodes(yamlFileNodes)
	if err != nil {
		t.Error(err)
	}

	if len(yamlFileNodes[os.Stdin.Name()]) != 1 {
		t.Error("failed to match length of filtered nodes")
	}
}

func TestFindResources(t *testing.T) {
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
	expectedResources := map[schema.GroupVersionKind]bool{
		{Group: "test.io", Version: "v1", Kind: "Tester"}:      true,
		{Group: "test.io", Version: "v1beta1", Kind: "Tester"}: true,
	}

	nodes, err := kio.FromBytes([]byte(manifests))
	if err != nil {
		t.Error(err)
	}

	resources, err := findResources(nodes)
	if err != nil {
		t.Error(err)
	}

	if !reflect.DeepEqual(resources, expectedResources) {
		t.Error("failed to find expected resources")
	}
}
