package main

import (
	"reflect"
	"testing"

	"github.com/dippynark/kfmt/discovery"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/kustomize/kyaml/kio"
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

	o := Options{createMissingNamespaces: true}

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
		t.Error("failed to find namespaces")
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
		t.Error("failed to find resources")
	}
}
