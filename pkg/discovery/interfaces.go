package discovery

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type ResourceInspector interface {
	// IsNamespaced returns true if the given GroupVersionKind is for a
	// namespace-scoped object.
	IsNamespaced(schema.GroupVersionKind) (bool, error)
	// AddGVKToScope adds GVK scope mapping to discovery
	AddGVKToScope(schema.GroupVersionKind, bool)
	// IsCoreGroup returns true if group is core
	IsCoreGroup(string) bool
}
