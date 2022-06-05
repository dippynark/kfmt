package utils

import (
    "fmt"
    "testing"

    "sigs.k8s.io/kustomize/kyaml/kio"
)

func TestGetAnnotations(t *testing.T) {
    manifests := `
apiVersion: v1
kind: Namespace
metadata:
  name: test
  annotations:
    foo: bah
---
apiVersion: v1
kind: Namespace
metadata:
  name: test
  annotations: {}
---
apiVersion: v1
kind: Namespace
metadata:
  name: test
`
    nodes, err := kio.FromBytes([]byte(manifests))
    if err != nil {
        t.Error(err)
    }

    if len(nodes) != 3 {
        t.Error("failed to ingest manifests")
    }

    annotations, err := GetAnnotations(nodes[0])
    if err != nil {
        t.Error(err)
    }

    value, ok := annotations["foo"]
    if !ok {
        t.Error("failed to retrieve annotation key foo")
    }

    if value != "bah" {
        t.Error("failed to retrieve annotation value bah")
    }

    if len(annotations) > 1 {
        t.Error("exactly one annotation was expected")
    }

    annotations, err = GetAnnotations(nodes[1])
    if err != nil {
        t.Error(err)
    }

    if len(annotations) > 0 {
        t.Error("exactly zero annotations were expected")
    }

    annotations, err = GetAnnotations(nodes[2])
    if err != nil {
        t.Error(err)
    }

    if len(annotations) > 0 {
        t.Error("exactly zero annotations were expected")
    }
}

func TestGetNamespace(t *testing.T) {
    manifests := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: test
  namespace: foo
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: test
`
    nodes, err := kio.FromBytes([]byte(manifests))
    if err != nil {
        t.Error(err)
    }

    if len(nodes) != 2 {
        t.Error("failed to ingest manifests")
    }

    namespace, err := GetNamespace(nodes[0])
    if err != nil {
        t.Error(err)
    }

    if namespace != "foo" {
        t.Error("failed to retrieve namespace foo")
    }

    namespace, err = GetNamespace(nodes[1])
    if err != nil {
        t.Error(err)
    }

    if namespace != "" {
        t.Error(fmt.Sprintf("found unknown namespace %s", namespace))
    }
}

func TestGetName(t *testing.T) {
    manifests := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: test
---
apiVersion: v1
kind: ConfigMap
`
    nodes, err := kio.FromBytes([]byte(manifests))
    if err != nil {
        t.Error(err)
    }

    if len(nodes) != 2 {
        t.Error("failed to ingest manifests")
    }

    name, err := GetName(nodes[0])
    if err != nil {
        t.Error(err)
    }

    if name != "test" {
        t.Error("failed to retrieve name test")
    }

    name, err = GetName(nodes[1])
    if err == nil {
        t.Error("expected error due to missing name")
    }
}

func TestGetAPIVersion(t *testing.T) {
    manifests := `
apiVersion: v1
kind: ConfigMap
---
kind: ConfigMap
`
    nodes, err := kio.FromBytes([]byte(manifests))
    if err != nil {
        t.Error(err)
    }

    if len(nodes) != 2 {
        t.Error("failed to ingest manifests")
    }

    apiVersion, err := GetAPIVersion(nodes[0])
    if err != nil {
        t.Error(err)
    }

    if apiVersion != "v1" {
        t.Error("failed to retrieve apiVersion v1")
    }

    apiVersion, err = GetAPIVersion(nodes[1])
    if err == nil {
        t.Error("expected error due to missing apiVersion")
    }
}

func TestGetKind(t *testing.T) {
    manifests := `
apiVersion: v1
kind: ConfigMap
---
apiVersion: v1
`
    nodes, err := kio.FromBytes([]byte(manifests))
    if err != nil {
        t.Error(err)
    }

    if len(nodes) != 2 {
        t.Error("failed to ingest manifests")
    }

    kind, err := GetKind(nodes[0])
    if err != nil {
        t.Error(err)
    }

    if kind != "ConfigMap" {
        t.Error("failed to retrieve kind ConfigMap")
    }

    kind, err = GetKind(nodes[1])
    if err == nil {
        t.Error("expected error due to missing kind")
    }
}