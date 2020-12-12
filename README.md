# config-cleaver

This tool takes inspiration from a number of open and closed source tools:

- [manifest-splitter](https://github.com/munnerz/manifest-splitter)
- [nomos](https://cloud.google.com/anthos-config-management/docs/how-to/nomos-command)
- [jx gitops split](https://github.com/jenkins-x/jx-gitops/blob/master/docs/cmd/jx-gitops_split.md)
- [jx gitops rename](https://github.com/jenkins-x/jx-gitops/blob/master/docs/cmd/jx-gitops_rename.md)
- [jx gitops helmfile move](https://github.com/jenkins-x/jx-gitops/blob/master/docs/cmd/jx-gitops_helmfile_move.md)

## Usage

```sh
cleave --input-dir input1 --input-dir input2 --output-dir output
```
