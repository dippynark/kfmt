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
		resources: resources,
	}
}

func (l *LocalResourceInspector) IsNamespaced(gvk schema.GroupVersionKind) (bool, error) {
	namespaced, ok := l.resources[gvk]
	if !ok {
		return false, fmt.Errorf("could not find REST mapping for resource %v", gvk.String())
	}

	return namespaced, nil
}

func (l *LocalResourceInspector) AddResource(gvk schema.GroupVersionKind, namespaced bool) {
	l.resources[gvk] = namespaced
}

var _ ResourceInspector = &LocalResourceInspector{}
