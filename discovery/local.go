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
	gvkToScope map[schema.GroupVersionKind]bool
}

func NewLocalResourceInspector() (*LocalResourceInspector, error) {
	cachedGVKToScope, err := parseCachedAPIResources()
	if err != nil {
		return nil, err
	}

	gvkToScope := copyMap(coreGVKToScope)
	for k, v := range cachedGVKToScope {
		gvkToScope[k] = v
	}

	return &LocalResourceInspector{
		gvkToScope: gvkToScope,
	}, nil
}

func (l *LocalResourceInspector) IsNamespaced(gvk schema.GroupVersionKind) (bool, error) {
	namespaced, ok := l.gvkToScope[gvk]
	if !ok {
		return false, fmt.Errorf("could not find REST mapping for resource %v", gvk.String())
	}

	return namespaced, nil
}

func (l *LocalResourceInspector) IsCoreGroup(group string) bool {
	for gvk, _ := range coreGVKToScope {
		if group == gvk.Group {
			return true
		}
	}

	return false
}

func (l *LocalResourceInspector) AddGVKToScope(gvk schema.GroupVersionKind, namespaced bool) {
	l.gvkToScope[gvk] = namespaced
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
	cachedGVKToScope := map[schema.GroupVersionKind]bool{}

	// TODO: allow paths to be user-specified
	apiResourcesFile := "api-resources.txt"
	apiVersionsFile := "api-versions.txt"

	if _, err := os.Stat(apiResourcesFile); os.IsNotExist(err) {
		return cachedGVKToScope, nil
	}

	file, err := os.Open(apiResourcesFile)
	if err != nil {
		return cachedGVKToScope, err
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
			return cachedGVKToScope, err
		}
		namespaced, err := strconv.ParseBool(words[len(words)-2])
		if err != nil {
			return cachedGVKToScope, err
		}
		kind := words[len(words)-1]

		gvk := schema.GroupVersionKind{
			Group:   gv.Group,
			Version: gv.Version,
			Kind:    kind,
		}
		cachedGVKToScope[gvk] = namespaced
	}

	if _, err := os.Stat(apiVersionsFile); os.IsNotExist(err) {
		return cachedGVKToScope, nil
	}

	file, err = os.Open(apiVersionsFile)
	if err != nil {
		return cachedGVKToScope, err
	}
	defer file.Close()

	scanner = bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		gv, err := schema.ParseGroupVersion(line)
		if err != nil {
			return cachedGVKToScope, err
		}

		for gvk, namespaced := range cachedGVKToScope {
			if gvk.Group == gv.Group {
				newGVK := gvk
				newGVK.Version = gv.Version
				cachedGVKToScope[newGVK] = namespaced
			}
		}
	}

	return cachedGVKToScope, nil
}
