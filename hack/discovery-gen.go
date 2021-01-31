package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bradfitz/slice"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func main() {

	// TODO: map from group/kind to namespaced (ignore version)
	// schema.GroupKind
	gvkNamespaced, err := parseGVKNamespacedMapping()
	if err != nil {
		fmt.Print(err)
		os.Exit(1)
	}

	file, err := os.OpenFile(os.Args[len(os.Args)-1], os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		os.Exit(1)
	}

	file.WriteString(fmt.Sprintf("package discovery\n\n"))
	file.WriteString(fmt.Sprintf("import \"k8s.io/apimachinery/pkg/runtime/schema\"\n\n"))
	file.WriteString(fmt.Sprintf("var coreResources = map[schema.GroupVersionKind]bool{\n"))

	var keys []schema.GroupVersionKind
	for k := range gvkNamespaced {
		keys = append(keys, k)
	}
	slice.Sort(keys[:], func(i, j int) bool {
		if keys[i].Group != keys[j].Group {
			return keys[i].Group < keys[j].Group
		}
		if keys[i].Version != keys[j].Version {
			return keys[i].Version < keys[j].Version
		}
		return keys[i].Kind < keys[j].Kind
	})

	for _, k := range keys {
		file.WriteString(fmt.Sprintf("  {Group: \"%s\", Version: \"%s\", Kind: \"%s\"}: %s,\n", k.Group, k.Version, k.Kind, strconv.FormatBool(gvkNamespaced[k])))
	}

	file.WriteString(fmt.Sprintf("}"))

	if err := file.Close(); err != nil {
		os.Exit(1)
	}
}

func extractGVKNamespacedMapping(typesFileName, group, version string) (map[schema.GroupVersionKind]bool, error) {
	gvkNamespaced := map[schema.GroupVersionKind]bool{}

	file, err := os.Open(typesFileName)
	if err != nil {
		return gvkNamespaced, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		// Find kind marker
		if line == "// +genclient" {
			gvk := schema.GroupVersionKind{
				Group:   group,
				Version: version,
			}
			// Determine whether type is Namespaced
			namespaced := true
			for scanner.Scan() {
				line := scanner.Text()
				// Break if line is not a comment
				if !strings.HasPrefix(line, "//") {
					break
				}
				if line == "// +genclient:nonNamespaced" {
					namespaced = false
					break
				}
			}
			// Extract kind
			for scanner.Scan() {
				line := scanner.Text()
				// Break if line is not a comment, whitespace or a type definition
				if !strings.HasPrefix(line, "//") && line != "" && !strings.HasPrefix(line, "type ") {
					break
				}
				if strings.HasPrefix(line, "type ") {
					gvk.Kind = strings.Split(line, " ")[1]
				}
			}
			if gvk.Kind == "" {
				return gvkNamespaced, fmt.Errorf("Unable to find kind: %s", typesFileName)
			}
			gvkNamespaced[gvk] = namespaced
		}
	}

	return gvkNamespaced, nil
}

func extractSubstring(registerFileName, prefix, suffix string) (string, error) {
	file, err := os.Open(registerFileName)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, prefix) && strings.HasSuffix(line, suffix) {
			return strings.TrimSuffix(strings.TrimPrefix(line, prefix), suffix), nil
		}
	}

	return "", fmt.Errorf("failed to find substring: %s %s %s", registerFileName, prefix, suffix)
}

func parseGVKNamespacedMapping() (map[schema.GroupVersionKind]bool, error) {

	gvkNamespaced := map[schema.GroupVersionKind]bool{}

	for _, path := range os.Args[len(os.Args)-4:len(os.Args)-1] {
		err := filepath.Walk(path,
			func(fileName string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if strings.HasSuffix(fileName, "/types.go") {
					group, err := extractSubstring(strings.TrimSuffix(fileName, "/types.go")+"/register.go", "const GroupName = \"", "\"")
					if err != nil {
						// Ignore errors here due to apiextensions-apiserver/examples/client-go/pkg/apis/cr/v1/register.go
						// return err
						return nil
					}
					version, err := extractSubstring(strings.TrimSuffix(fileName, "/types.go")+"/register.go", "var SchemeGroupVersion = schema.GroupVersion{Group: GroupName, Version: \"", "\"}")
					if err != nil {
						// Ignore errors here due to kube-aggregator/pkg/apis/apiregistration/register.go
						// return err
						return nil
					}
					extractedGVKNamespaced, err := extractGVKNamespacedMapping(fileName, group, version)
					if err != nil {
						return err
					}
					for k, v := range extractedGVKNamespaced {
						gvkNamespaced[k] = v
					}
				}
				return nil
			})
		if err != nil {
			return gvkNamespaced, err
		}
	}

	return gvkNamespaced, nil
}
