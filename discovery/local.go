package discovery

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// LocalResourceInspector implements ResourceInspector using local configs
type LocalResourceInspector struct {
	resources map[schema.GroupVersionKind]bool
}

func NewLocalResourceInspector() *LocalResourceInspector {
	return &LocalResourceInspector{
		resources: copyMap(coreResources),
	}
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
