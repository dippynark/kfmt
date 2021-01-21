package discovery

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/restmapper"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LocalResourceInspector implements ResourceInspector using local configs.
type LocalResourceInspector struct {
	mapper meta.RESTMapper
}

func NewLocalResourceInspector() (*LocalResourceInspector, error) {
	mapper := restmapper.NewDiscoveryRESTMapper(generateGroupResources())

	return &LocalResourceInspector{
		mapper: mapper,
	}, nil
}

func (l *LocalResourceInspector) IsNamespaced(gvk schema.GroupVersionKind) (bool, error) {
	mapping, err := l.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return false, fmt.Errorf("could not find REST mapping for resource %v: %w", gvk.String(), err)
	}

	return mapping.Scope.Name() == meta.RESTScopeNameNamespace, nil
}

var _ ResourceInspector = &LocalResourceInspector{}

func generateGroupResources() []*restmapper.APIGroupResources {
	groupResources := []*restmapper.APIGroupResources{}

	for gvk, namespaced := range gvkNamespaced {
		groupFound := false

		plural, singular := meta.UnsafeGuessKindToResource(gvk)
		apiResource := metav1.APIResource{
			Name:         plural.Resource,
			SingularName: singular.Resource,
			Namespaced:   namespaced,
			Group:        gvk.Group,
			Version:      gvk.Version,
			Kind:         gvk.Kind,
		}

		groupVersion := schema.GroupVersion{
			Group:   gvk.Group,
			Version: gvk.Version,
		}
		version := metav1.GroupVersionForDiscovery{GroupVersion: groupVersion.String(), Version: gvk.Version}

		// Search for existing group
		for i, groupResource := range groupResources {
			if groupResource.Group.Name == gvk.Group {
				groupFound = true
				// Append group version
				groupResources[i].Group.Versions = append(groupResources[i].Group.Versions, version)
				// Append API resource
				groupResource.VersionedResources[gvk.Version] = append(groupResource.VersionedResources[gvk.Version], apiResource)
			}
		}

		if !groupFound {
			apiGroup := metav1.APIGroup{
				Name:             gvk.Group,
				Versions:         []metav1.GroupVersionForDiscovery{version},
				PreferredVersion: version,
			}
			groupResource := &restmapper.APIGroupResources{
				Group: apiGroup,
			}
			groupResource.VersionedResources = map[string][]metav1.APIResource{}
			groupResource.VersionedResources[gvk.Version] = append(groupResource.VersionedResources[gvk.Version], apiResource)
			groupResources = append(groupResources, groupResource)
		}
	}

	return groupResources
}
