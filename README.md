# kfmt

This tool takes input directories containing Kubernetes configs and structures them into a canonical
format.

Inspiration is taken from a number of other tools:

- [manifest-splitter](https://github.com/munnerz/manifest-splitter)
- [nomos](https://cloud.google.com/anthos-config-management/docs/how-to/nomos-command)
- [jx gitops split](https://github.com/jenkins-x/jx-gitops/blob/master/docs/cmd/jx-gitops_split.md)
- [jx gitops
  rename](https://github.com/jenkins-x/jx-gitops/blob/master/docs/cmd/jx-gitops_rename.md)
- [jx gitops helmfile
  move](https://github.com/jenkins-x/jx-gitops/blob/master/docs/cmd/jx-gitops_helmfile_move.md)

## Use Case

GitOps tools such as [Flux](https://github.com/fluxcd/flux) and [Anthos Config
Management](https://cloud.google.com/anthos/config-management) sync configs from a Git repository to
a Kubernetes cluster. kfmt allows you to take the hydrated configs to be synced and reformat them
into a canonical format which these GitOps tools can then be pointed at. When changes are made to
these configs, having them formatted in this canonical format makes it easier for a human to review
the changes that are going to be made to the cluster and ensures there are no clashes. The canonical
format looks as follows:

```sh
# Directory to be synced
output
  # Directory containing non-namespaced resources
  cluster
    # Each non-namespaced resource is moved into a directory named after its kind
    clusterrolebinding
      # Files are named after the resource name and kind
      cert-manager-cainjector.yaml
      cert-manager-controller-certificates.yaml
      ...
    clusterroles
    namespaces
    ...
  # Directory containing namespaced resources
  namespaces
    # Each Namespace is given its own directory
    cert-manager
      cert-manager-cainjector-deployment.yaml
      ...
    kube-system
    ...
```

## Usage

```text
kfmt organises Kubernetes configs into a canonical format

Usage:
  kfmt [flags]

Flags:
      --clean                Remove namespace field from non-namespaced resources
      --comment              Comment each output file with the absolute path of the corresponding input file
      --discovery            Use API Server for discovery
  -f, --filter stringArray   Filter kind.group from output configs (e.g. Deployment.apps or Secret)
  -h, --help                 Help for kfmt
  -i, --input stringArray    Input files or directories containing hydrated configs. If no input is specified /dev/stdin will be used
  -k, --kubeconfig string    Absolute path to the kubeconfig file used for discovery (default "/Users/luke/.kube/config")
  -n, --namespace string     Set namespace field if missing from namespaced resources
  -o, --output string        Output directory to write structured configs
      --overwrite            Overwrite existing output files
      --remove               Remove processed input files
      --strict               Require namespace is not set for non-namespaced resources
```

Namespaced resources in any input can be annotated as follows:

```
kfmt.dev/namespaces: "namespace1,namespace2,..."
```

The resource will be copied into each named Namespace. Note that each Namespace must be present in
the configs being processed, either due to a Namespace resource being defined or any Namespaced
resource being in that Namespace. Alternatively, the special value `*` can be used and the resource
will be copied into every Namespace. Prefixing a Namespace name with `-` excludes that Namespace.

## Example

The following sequence of targets builds kfmt, downloads the
[cert-manager](https://github.com/jetstack/cert-manager) release manifests and formats them into the
canonical format.

```sh
make build test
```
