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

## Usage

```text
kfmt organises Kubernetes configs into a canonical structure.

Usage:
  kfmt [flags]

Flags:
  -d, --discovery               Use API Server for discovery
  -h, --help                    help for kfmt
  -i, --input-dir stringArray   Directory containing hydrated configs
  -u, --kubeconfig string       Absolute path to the kubeconfig file used for discovery (default "/Users/luke/.kube/config")
  -o, --output-dir string       Output directory
```
