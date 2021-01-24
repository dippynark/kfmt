package discovery

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type ResourceInspector interface {
	// IsNamespaced returns true if the given GroupVersionKind is for a
	// namespace-scoped object.
	IsNamespaced(schema.GroupVersionKind) (bool, error)
	// AddResource adds a GVK namespaced mapping to discovery
	AddResource(schema.GroupVersionKind, bool)
	// IsCoreCollision returns true if kind.group clashes with core resource
	IsCoreCollision(kind, group string) bool
}
