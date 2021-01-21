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
	mapper *restmapper.DeferredDiscoveryRESTMapper
}

func NewAPIServerResourceInspector(cfg *rest.Config) (*APIServerResourceInspector, error) {
	cl, err := kdiscov.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, err
	}

	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(cl))

	return &APIServerResourceInspector{
		mapper: mapper,
	}, nil
}

func (a *APIServerResourceInspector) IsNamespaced(gvk schema.GroupVersionKind) (bool, error) {
	mapping, err := a.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return false, fmt.Errorf("could not find REST mapping for resource %v: %w", gvk.String(), err)
	}

	return mapping.Scope.Name() == meta.RESTScopeNameNamespace, nil
}

var _ ResourceInspector = &APIServerResourceInspector{}
