package discovery

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// LocalResourceInspector implements ResourceInspector using local manifests
type LocalResourceInspector struct {
	resources map[schema.GroupVersionKind]bool
}

func NewLocalResourceInspector() (*LocalResourceInspector, error) {
	cachedResources, err := parseCachedAPIResources()
	if err != nil {
		return nil, err
	}

	resources := copyMap(coreResources)
	for k, v := range cachedResources {
		resources[k] = v
	}

	return &LocalResourceInspector{
		resources: resources,
	}, nil
}

func (l *LocalResourceInspector) IsNamespaced(gvk schema.GroupVersionKind) (bool, error) {
	namespaced, ok := l.resources[gvk]
	if !ok {
		return false, fmt.Errorf("could not find REST mapping for resource %v", gvk.String())
	}

	return namespaced, nil
}

func (l *LocalResourceInspector) IsCoreGroup(group string) bool {
	for gvk, _ := range coreResources {
		if group == gvk.Group {
			return true
		}
	}

	return false
}

func (l *LocalResourceInspector) AddResource(gvk schema.GroupVersionKind, namespaced bool) {
	l.resources[gvk] = namespaced
}

var _ ResourceInspector = &LocalResourceInspector{}

func copyMap(m map[schema.GroupVersionKind]bool) map[schema.GroupVersionKind]bool {
	cp := make(map[schema.GroupVersionKind]bool)
	for k, v := range m {
		cp[k] = v
	}

	return cp
}

func parseCachedAPIResources() (map[schema.GroupVersionKind]bool, error) {
	cachedResources := map[schema.GroupVersionKind]bool{}

	// TODO: allow paths to be user-specified
	apiResourcesFile := "api-resources.txt"
	apiVersionsFile := "api-versions.txt"

	if _, err := os.Stat(apiResourcesFile); os.IsNotExist(err) {
		return cachedResources, nil
	}

	file, err := os.Open(apiResourcesFile)
	if err != nil {
		return cachedResources, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		words := strings.Fields(line)
		if len(words) == 0 {
			continue
		}
		if words[0] == "NAME" {
			continue
		}

		gv, err := schema.ParseGroupVersion(words[len(words)-3])
		if err != nil {
			return cachedResources, err
		}
		namespaced, err := strconv.ParseBool(words[len(words)-2])
		if err != nil {
			return cachedResources, err
		}
		kind := words[len(words)-1]

		gvk := schema.GroupVersionKind{
			Group:   gv.Group,
			Version: gv.Version,
			Kind:    kind,
		}
		cachedResources[gvk] = namespaced
	}

	if _, err := os.Stat(apiVersionsFile); os.IsNotExist(err) {
		return cachedResources, nil
	}

	file, err = os.Open(apiVersionsFile)
	if err != nil {
		return cachedResources, err
	}
	defer file.Close()

	scanner = bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		gv, err := schema.ParseGroupVersion(line)
		if err != nil {
			return cachedResources, err
		}

		for gvk, namespaced := range cachedResources {
			if gvk.Group == gv.Group {
				newGVK := gvk
				newGVK.Version = gv.Version
				cachedResources[newGVK] = namespaced
			}
		}
	}

	return cachedResources, nil
}
