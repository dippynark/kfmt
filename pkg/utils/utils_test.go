package utils

import (
    "testing"

    "sigs.k8s.io/kustomize/kyaml/kio"
)

func TestGetAnnotations(t *testing.T) {
    namespace := `
apiVersion: v1
kind: Namespace
metadata:
  name: test
  annotations:
    foo: bah
`
    nodes, err := kio.FromBytes([]byte(namespace))
    if err != nil {
        t.Error(err)
    }

    if len(nodes) != 1 {
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
        t.Error("found too many annotations")
    }
}
