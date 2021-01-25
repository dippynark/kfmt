package discovery

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kdiscov "k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
)

// APIServerResourceInspector implements ResourceInspector using the Kubernetes
// discovery API.
// It relies on a Kubernetes apiserver that has discovery information for all
// inputted resource types.
type APIServerResourceInspector struct {
	localResourceInspector *LocalResourceInspector
	mapper                 *restmapper.DeferredDiscoveryRESTMapper
}

func NewAPIServerResourceInspector(cfg *rest.Config) (*APIServerResourceInspector, error) {
	cl, err := kdiscov.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, err
	}

	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(cl))
	localResourceInspector, err := NewLocalResourceInspector()
	if err != nil {
		return nil, err
	}

	return &APIServerResourceInspector{
		mapper:                 mapper,
		localResourceInspector: localResourceInspector,
	}, nil
}

func (a *APIServerResourceInspector) IsNamespaced(gvk schema.GroupVersionKind) (bool, error) {
	// First check local discovery...
	namespaced, err := a.localResourceInspector.IsNamespaced(gvk)
	// Ignore error if local discovery is unsuccessful
	// TODO: use multi-error struct
	if err == nil {
		return namespaced, nil
	}

	// ...now check API Server discovery
	mapping, err := a.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return false, fmt.Errorf("could not find REST mapping for resource %v: %w", gvk.String(), err)
	}

	return mapping.Scope.Name() == meta.RESTScopeNameNamespace, nil
}

func (a *APIServerResourceInspector) IsCoreGroup(group string) bool {
	return a.localResourceInspector.IsCoreGroup(group)
}

func (a *APIServerResourceInspector) AddResource(gvk schema.GroupVersionKind, namespaced bool) {
	a.localResourceInspector.AddResource(gvk, namespaced)
}

var _ ResourceInspector = &APIServerResourceInspector{}
