module github.com/dippynark/kfmt

go 1.15

//https://github.com/kubernetes/kubernetes/issues/79384#issuecomment-505627280
replace (
	k8s.io/api => k8s.io/api v0.20.2
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.20.2
	k8s.io/apimachinery => k8s.io/apimachinery v0.20.2
	k8s.io/apiserver => k8s.io/apiserver v0.20.2
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.20.2
	k8s.io/client-go => k8s.io/client-go v0.20.2
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.20.2
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.20.2
	k8s.io/code-generator => k8s.io/code-generator v0.20.2
	k8s.io/component-base => k8s.io/component-base v0.20.2
	k8s.io/component-helpers => k8s.io/component-helpers v0.20.2
	k8s.io/controller-manager => k8s.io/controller-manager v0.20.2
	k8s.io/cri-api => k8s.io/cri-api v0.20.2
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.20.2
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.20.2
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.20.2
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.20.2
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.20.2
	k8s.io/kubectl => k8s.io/kubectl v0.20.2
	k8s.io/kubelet => k8s.io/kubelet v0.20.2
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.20.2
	k8s.io/metrics => k8s.io/metrics v0.20.2
	k8s.io/mount-utils => k8s.io/mount-utils v0.20.2
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.20.2
)

require (
	github.com/Azure/go-autorest/autorest v0.11.17 // indirect
	github.com/Azure/go-autorest/autorest/adal v0.9.10 // indirect
	github.com/go-logr/logr v0.2.1 // indirect
	github.com/go-openapi/spec v0.19.7 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/googleapis/gnostic v0.4.2 // indirect
	github.com/imdario/mergo v0.3.11 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/niemeyer/pretty v0.0.0-20200227124842-a10e7caefd8e // indirect
	github.com/pkg/errors v0.9.1
	github.com/spf13/afero v1.2.2
	github.com/spf13/cobra v1.1.1
	github.com/stretchr/testify v1.6.1
	golang.org/x/sys v0.0.0-20220520151302-bc2c85ada10a // indirect
	golang.org/x/text v0.3.5 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	gopkg.in/check.v1 v1.0.0-20200227125254-8fa46927fb4f // indirect
	gopkg.in/yaml.v3 v3.0.0-20200615113413-eeeca48fe776 // indirect
	k8s.io/api v0.20.2
	k8s.io/apiextensions-apiserver v0.20.4
	k8s.io/apimachinery v0.20.2
	k8s.io/client-go v0.20.2
	sigs.k8s.io/kustomize/kyaml v0.10.6
)
