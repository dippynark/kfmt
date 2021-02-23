package main

import (
	"strings"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

var quotes = []string{"'", "\""}

func getAnnotations(node *yaml.RNode) (map[string]string, error) {
	annotations := map[string]string{}

	valueNode, err := node.Pipe(yaml.Lookup("metadata", "annotations"))
	if err != nil {
		return annotations, err
	}

	m := valueNode.Map()
	for k, v := range m {
		// Ignore nil annotations
		if v == nil {
			continue
		}
		annotations[k] = v.(string)
	}

	return annotations, nil
}

func getNamespace(node *yaml.RNode) (string, error) {
	namespace, err := getStringField(node, "metadata", "namespace")
	if err != nil {
		return "", err
	}

	return namespace, nil
}

func getName(node *yaml.RNode) (string, error) {
	name, err := getStringField(node, "metadata", "name")
	if err != nil {
		return "", err
	}

	if name == "" {
		return "", errors.New("name is empty")
	}

	return name, nil
}

func getKind(node *yaml.RNode) (string, error) {
	kind, err := getStringField(node, "kind")
	if err != nil {
		return "", err
	}

	if kind == "" {
		return "", errors.New("kind is empty")
	}

	return kind, nil
}

func getAPIVersion(node *yaml.RNode) (string, error) {
	kind, err := getStringField(node, "apiVersion")
	if err != nil {
		return "", err
	}

	if kind == "" {
		return "", errors.New("apiVersion is empty")
	}

	return kind, nil
}

func getCRDGroup(node *yaml.RNode) (string, error) {
	group, err := getStringField(node, "spec", "group")
	if err != nil {
		return "", err
	}

	if group == "" {
		return "", errors.New("group is empty")
	}

	return group, nil
}

func getCRDKind(node *yaml.RNode) (string, error) {
	kind, err := getStringField(node, "spec", "names", "kind")
	if err != nil {
		return "", err
	}

	if kind == "" {
		return "", errors.New("CRD kind is empty")
	}

	return kind, nil
}

func getCRDScope(node *yaml.RNode) (string, error) {
	scope, err := getStringField(node, "spec", "scope")
	if err != nil {
		return "", err
	}

	if scope == "" {
		return "", errors.New("scope is empty")
	}

	return scope, nil
}

func getCRDVersions(node *yaml.RNode) ([]string, error) {
	valueNode, err := node.Pipe(yaml.Lookup("spec", "versions"))
	if err != nil {
		return nil, err
	}

	versions, err := valueNode.ElementValues("name")
	if err != nil {
		return nil, err
	}

	if len(versions) > 0 {
		return versions, nil
	}

	version, err := getStringField(node, "spec", "version")
	if err != nil {
		return nil, err
	}

	return []string{version}, nil
}

func getStringField(node *yaml.RNode, fields ...string) (string, error) {

	valueNode, err := node.Pipe(yaml.Lookup(fields...))
	if err != nil {
		return "", err
	}

	// Return empty string if value not found
	if valueNode == nil {
		return "", nil
	}

	value, err := valueNode.String()
	if err != nil {
		return "", nil
	}

	return trimSpaceAndQuotes(value), nil
}

// trimSpaceAndQuotes trims any whitespace and quotes around a value
func trimSpaceAndQuotes(value string) string {
	text := strings.TrimSpace(value)
	for _, q := range quotes {
		if strings.HasPrefix(text, q) && strings.HasSuffix(text, q) {
			return strings.TrimPrefix(strings.TrimSuffix(text, q), q)
		}
	}
	return text
}

func pluralise(lowercaseKind string) string {

	// e.g. ingress
	if strings.HasSuffix(lowercaseKind, "s") {
		return lowercaseKind + "es"
	}
	// e.g. networkpolicy
	if strings.HasSuffix(lowercaseKind, "cy") {
		return strings.TrimRight(lowercaseKind, "y") + "ies"
	}

	return lowercaseKind + "s"
}

func isWhitespaceOrComments(input string) bool {
	lines := strings.Split(input, "\n")
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t != "" && !strings.HasPrefix(t, "#") && !strings.HasPrefix(t, "--") {
			return false
		}
	}
	return true
}

func getGVK(node *yaml.RNode) (gvk schema.GroupVersionKind, err error) {
	apiVersion, err := getAPIVersion(node)
	if err != nil {
		return gvk, errors.Wrap(err, "failed to get apiVersion")
	}

	kind, err := getKind(node)
	if err != nil {
		return gvk, errors.Wrap(err, "failed to get kind")
	}

	gvk = schema.FromAPIVersionAndKind(apiVersion, kind)

	return gvk, nil
}
