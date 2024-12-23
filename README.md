# kfmt

kfmt takes input files and directories containing Kubernetes manifests and organises them into a
standard format:

```t
# Output directory
output
|   # Directory containing cluster scoped resources
├── cluster
|   |   # Each cluster scoped resource is given a directory named after its plural and group
│   └── <plural>.<group>
|       |   # Files are named after the resource name
│       └── <name>.yaml
|   # Directory containing namespace scoped resources
└── namespaces
    |   # Each Namespace is given its own directory
    └── <namespace>
        |   # Files are named after the resource name, kind and group
        └── <kind>.<group>-<name>.yaml
```

Inspiration is taken from a number of other tools:

- [manifest-splitter](https://github.com/munnerz/manifest-splitter)
- [nomos](https://cloud.google.com/anthos-config-management/docs/how-to/nomos-command)
- [jx gitops split](https://github.com/jenkins-x/jx-gitops/blob/master/docs/cmd/jx-gitops_split.md)
- [jx gitops
  rename](https://github.com/jenkins-x/jx-gitops/blob/master/docs/cmd/jx-gitops_rename.md)
- [jx gitops helmfile
  move](https://github.com/jenkins-x/jx-gitops/blob/master/docs/cmd/jx-gitops_helmfile_move.md)

## Use Cases

kfmt is useful in any situation where it's beneficial to have a collection of manifests organised
into a standard format. This could be to tidy up a large collection of manifests or just to make
them easier to browse.

GitOps tools such as [Flux](https://github.com/fluxcd/flux2) and [Anthos Config
Management](https://cloud.google.com/anthos/config-management) that sync manifests from a Git
repository could also benefit from kfmt by running it as a final step in CI, taking in all the
manifests to be synced and verifying there are no clashes. Using the `kfmt.dev/namespaces`
annotation can also be used to copy policy resources across Namespaces and having a standard format
may make any changes easier to review.

## Installation

```sh
GO111MODULE=on go get github.com/dippynark/kfmt
```

kfmt is also distributed as a [binary](https://github.com/dippynark/kfmt/releases) and a [Docker
image](https://hub.docker.com/repository/docker/dippynark/kfmt).

## Usage

The simplest usage is to format manifests coming from stdin:

```sh
cat <<EOF | kfmt -o output
apiVersion: v1
kind: Namespace
metadata:
  name: test
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: test
---
apiVersion: v1
kind: Secret
metadata:
  name: test
  namespace: test
EOF
```

The full help text is show below:

```text
kfmt organises Kubernetes manifests into a standard format.

Usage:
  kfmt [flags]

Flags:
      --clean                       Remove metadata.namespace field from non-namespaced resources
      --comment                     Comment each output file with the path of the corresponding input file
      --create-missing-namespaces   Create missing Namespace manifests
  -d, --discovery                   Use API Server for discovery
  -f, --filter stringArray          Filter Kind.group from output manifests (e.g. Deployment.apps or Secret)
  -g, --gvk-scope stringArray       Add GVK scope mapping Kind.group/version:Cluster or Kind.group/version:Namespaced to discovery
  -h, --help                        Print help text
  -i, --input stringArray           Input files or directories containing manifests. If no input is specified /dev/stdin will be used
  -k, --kubeconfig string           Path to the kubeconfig file used for discovery (default "/.kube/config")
  -n, --namespace string            Set metadata.namespace field if missing from namespaced resources (default "default")
  -o, --output string               Output directory to write organised manifests
      --overwrite                   Overwrite existing output files
      --remove                      Remove processed input files
      --strict                      Require metadata.namespace field is not set for non-namespaced resources
  -v, --version                     Print version
```

Namespaced resources can be annotated as follows:

```
kfmt.dev/namespaces: "namespace1,namespace2,..."
```

The resource will be copied into each named Namespace that does already contain a matching resource
of the same name. Note that each Namespace must be present in the manifests being processed, either
due to a Namespace resource being defined or any namespaced resource being in that Namespace.
Alternatively, the special value `*` can be used and the resource will be copied into every
Namespace; prefixing a Namespace name with `-` excludes that Namespace.

### Discovery

kfmt needs to know whether a particular
[GVK](https://book.kubebuilder.io/cronjob-tutorial/gvks.html) maps to a namespaced resource or not
to know how to organise the input manifests. kfmt understands core Kubernetes resources and supports
discovering mappings for custom resources by reading CRDs in the input manifests, using the
`--gvk-scope` flag or reading cached discovery information, which can be produced by writing it to
disk:

```sh
kubectl api-resources > api-resources.txt
```

Similarly, this cached discovery information can be augmented with all available versions:

```sh
kubectl api-versions > api-versions.txt
```

In addition, kfmt supports the `--discovery` flag to enable use of the Kubernetes discovery API.
kfmt will only attempt to use the Kubernetes discovery API if the required discovery information is
not provided using another method.
