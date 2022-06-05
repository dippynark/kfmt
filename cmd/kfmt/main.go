package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/util/homedir"
)

// Set to `git describe --tags`
var version = "v0.0.0"

const (
	// Namespaces to copy resource into
	annotationNamespacesKey = "kfmt.dev/namespaces"
	annotationNamespacesAll = "*"

	kubeconfigEnvVar = "KUBECONFIG"

	manifestSeparator = "---\n"

	nonNamespacedDirectory = "cluster"
	namespacedDirectory    = "namespaces"

	defaultFilePerms      = 0644
	defaultDirectoryPerms = 0755
)

func main() {
	o := &options{}

	cmd := &cobra.Command{
		Use:   "kfmt",
		Short: "kfmt organises Kubernetes manifests into a standard format.",
		Run: func(cmd *cobra.Command, args []string) {
			err := o.run()
			if err != nil {
				fmt.Fprintf(os.Stderr, "kfmt: %s\n", err)
			}
			os.Exit(1)
		},
	}

	cmd.Flags().BoolP("help", "h", false, "Print help text")
	cmd.Flags().BoolVarP(&o.version, "version", "v", false, "Print version")
	cmd.Flags().StringArrayVarP(&o.inputs, "input", "i", []string{}, fmt.Sprintf("Input files or directories containing manifests. If no input is specified %s will be used", os.Stdin.Name()))
	cmd.Flags().StringVarP(&o.output, "output", "o", "", "Output directory to write organised manifests")
	cmd.Flags().StringArrayVarP(&o.filters, "filter", "f", []string{}, "Filter Kind.group from output manifests (e.g. Deployment.apps or Secret)")
	cmd.Flags().StringArrayVarP(&o.gvkScopes, "gvk-scope", "g", []string{}, "Add GVK scope mapping Kind.group/version:Cluster or Kind.group/version:Namespaced to discovery")
	cmd.Flags().StringVarP(&o.namespace, "namespace", "n", corev1.NamespaceDefault, "Set metadata.namespace field if missing from namespaced resources")
	cmd.Flags().BoolVar(&o.clean, "clean", false, "Remove metadata.namespace field from non-namespaced resources")
	cmd.Flags().BoolVar(&o.strict, "strict", false, "Require metadata.namespace field is not set for non-namespaced resources")
	cmd.Flags().BoolVar(&o.remove, "remove", false, "Remove processed input files")
	cmd.Flags().BoolVar(&o.comment, "comment", false, "Comment each output file with the path of the corresponding input file")
	cmd.Flags().BoolVar(&o.overwrite, "overwrite", false, "Overwrite existing output files")
	cmd.Flags().BoolVar(&o.createMissingNamespaces, "create-missing-namespaces", false, "Create missing Namespace manifests")
	cmd.Flags().BoolVarP(&o.discovery, "discovery", "d", false, "Use API Server for discovery")
	// https://github.com/kubernetes/client-go/blob/b72204b2445de5ac815ae2bb993f6182d271fdb4/examples/out-of-cluster-client-configuration/main.go#L45-L49
	if kubeconfigEnvVarValue := os.Getenv(kubeconfigEnvVar); kubeconfigEnvVarValue != "" {
		cmd.Flags().StringVarP(&o.kubeconfig, "kubeconfig", "k", kubeconfigEnvVarValue, "Path to the kubeconfig file used for discovery")
	} else if home := homedir.HomeDir(); home != "" {
		cmd.Flags().StringVarP(&o.kubeconfig, "kubeconfig", "k", filepath.Join(home, ".kube", "config"), "Path to the kubeconfig file used for discovery")
	} else {
		cmd.Flags().StringVarP(&o.kubeconfig, "kubeconfig", "k", "", "Path to the kubeconfig file used for discovery")
	}

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}
