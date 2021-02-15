package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/dippynark/kfmt/discovery"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/helper"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

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

var quotes = []string{"'", "\""}

type Options struct {
	output                  string
	inputs                  []string
	filters                 []string
	namespace               string
	clean                   bool
	strict                  bool
	remove                  bool
	comment                 bool
	overwrite               bool
	createMissingNamespaces bool
	discovery               bool
	kubeconfig              string
}

func main() {
	o := &Options{}

	cmd := &cobra.Command{
		Use:   "kfmt",
		Short: "kfmt organises Kubernetes manifests into a standard format.",
		Run: func(cmd *cobra.Command, args []string) {
			err := o.Run()
			helper.CheckErr(err)
		},
	}

	cmd.Flags().BoolP("help", "h", false, "Help for kfmt")
	cmd.Flags().StringArrayVarP(&o.inputs, "input", "i", []string{}, fmt.Sprintf("Input files or directories containing manifests. If no input is specified %s will be used", os.Stdin.Name()))
	cmd.Flags().StringVarP(&o.output, "output", "o", "", "Output directory to write organised manifests")
	cmd.Flags().StringArrayVarP(&o.filters, "filter", "f", []string{}, "Filter kind.group from output manifests (e.g. Deployment.apps or Secret)")
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

func (o *Options) Run() error {

	if o.output == "" {
		return errors.Errorf("output directory not specified")
	}

	// Initialise discovery to determine whether resources are namespaced or not
	resourceInspector, err := o.getResourceInspector()
	if err != nil {
		return err
	}

	// Find all YAML files specified as input
	yamlFiles, err := o.findYAMLFiles()
	if err != nil {
		return err
	}

	// Map input files to nodes (parsed YAML documents)
	yamlFileNodes, err := o.findYAMLFileNodes(yamlFiles)
	if err != nil {
		return err
	}

	// Add local CRDs to discovery
	err = o.localDiscovery(yamlFileNodes, resourceInspector)
	if err != nil {
		return err
	}

	// Find all Namespaces either declared as resources or appearing in the metadata.namespace field
	allNamespaces, err := o.findAllNamespaces(yamlFileNodes, resourceInspector)
	if err != nil {
		return err
	}

	// Apply resource filters
	err = o.filterNodes(yamlFileNodes)
	if err != nil {
		return err
	}

	// Namespace defaulting
	err = o.defaultNamespaces(yamlFileNodes, resourceInspector)
	if err != nil {
		return err
	}

	// Apply `kfmt.dev/namespaces` annotation
	err = o.mirrorNodes(yamlFileNodes, allNamespaces, resourceInspector)
	if err != nil {
		return err
	}

	// Write out nodes
	err = o.writeNodes(yamlFileNodes, resourceInspector)
	if err != nil {
		return err
	}

	// Remove processed files
	err = o.removeNodes(yamlFileNodes)
	if err != nil {
		return err
	}

	// Create missing Namespace manifests
	if err := o.createMissingNamespaceManifests(allNamespaces); err != nil {
		return err
	}

	return nil
}

func (o *Options) getResourceInspector() (discovery.ResourceInspector, error) {
	var resourceInspector discovery.ResourceInspector
	if o.discovery {
		restcfg, err := clientcmd.BuildConfigFromFlags("", o.kubeconfig)
		if err != nil {
			log.Fatalf("Failed to build kubernetes REST client config: %v", err)
		}
		resourceInspector, err = discovery.NewAPIServerResourceInspector(restcfg)
		if err != nil {
			log.Fatalf("Failed to construct APIServer backed resource inspector: %v", err)
		}
	} else {
		var err error
		resourceInspector, err = discovery.NewLocalResourceInspector()
		if err != nil {
			log.Fatalf("Failed to construct locally backed resource inspector: %v", err)
		}
	}

	return resourceInspector, nil
}

func (o *Options) findYAMLFileNodes(yamlFiles []string) (map[string][]*yaml.RNode, error) {
	yamlFileNodes := map[string][]*yaml.RNode{}
	for _, yamlFile := range yamlFiles {
		b, err := ioutil.ReadFile(yamlFile)
		if err != nil {
			return yamlFileNodes, err
		}
		newNodes, err := kio.FromBytes(b)
		if err != nil {
			return yamlFileNodes, err
		}
		yamlFileNodes[yamlFile] = newNodes
	}
	return yamlFileNodes, nil
}

func (o *Options) findYAMLFiles() ([]string, error) {
	var yamlFiles []string
	if len(o.inputs) > 0 {
		for _, input := range o.inputs {
			info, err := os.Stat(input)
			if err != nil {
				return yamlFiles, err
			}
			switch mode := info.Mode(); {
			case mode.IsDir():
				inputFiles, err := listYAMLFiles(input)
				if err != nil {
					return yamlFiles, err
				}
				yamlFiles = append(yamlFiles, inputFiles...)
			case mode.IsRegular():
				yamlFiles = append(yamlFiles, input)
			default:
				return yamlFiles, fmt.Errorf("%s is not a directory or regular file", input)
			}
		}
	} else {
		// Read from stdin if no input specified
		yamlFiles = []string{os.Stdin.Name()}
	}
	return yamlFiles, nil
}

func (o *Options) localDiscovery(yamlFileNodes map[string][]*yaml.RNode, resourceInspector discovery.ResourceInspector) error {
	for yamlFile, nodes := range yamlFileNodes {
		resources, err := findResources(nodes)
		if err != nil {
			return errors.Wrapf(err, "failed to find CRDs in %s", yamlFile)
		}
		for gvk, namespaced := range resources {
			resourceInspector.AddResource(gvk, namespaced)
		}
	}
	return nil
}

func (o *Options) findAllNamespaces(yamlFileNodes map[string][]*yaml.RNode, resourceInspector discovery.ResourceInspector) (map[string]struct{}, error) {
	allNamespaces := map[string]struct{}{}
	for yamlFile, nodes := range yamlFileNodes {
		newNamespaces, err := o.findNamespaces(nodes, resourceInspector)
		if err != nil {
			return allNamespaces, errors.Wrapf(err, "failed to find Namespaces in %s", yamlFile)
		}
		for k, v := range newNamespaces {
			allNamespaces[k] = v
		}
	}
	return allNamespaces, nil
}

func (o *Options) removeNodes(yamlFileNodes map[string][]*yaml.RNode) error {
	if o.remove {
		for yamlFile := range yamlFileNodes {
			// Ignore stdin
			if yamlFile == os.Stdin.Name() {
				continue
			}
			err := os.Remove(yamlFile)
			if err != nil {
				return errors.Wrapf(err, "failed to remove input file %s", yamlFile)
			}
		}
	}
	return nil
}

func (o *Options) writeNodes(yamlFileNodes map[string][]*yaml.RNode, resourceInspector discovery.ResourceInspector) error {
	for yamlFile, nodes := range yamlFileNodes {
		for _, node := range nodes {

			outputFile, err := o.getOutputFile(node, resourceInspector)
			if err != nil {
				return err
			}

			err = o.writeNode(yamlFile, outputFile, node)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (o *Options) mirrorNodes(yamlFileNodes map[string][]*yaml.RNode, allNamespaces map[string]struct{}, resourceInspector discovery.ResourceInspector) error {
	for yamlFile, nodes := range yamlFileNodes {
		newNodes := []*yaml.RNode{}
		for _, node := range nodes {

			gvk, err := getGVK(node)
			if err != nil {
				return err
			}

			isNamespaced, err := resourceInspector.IsNamespaced(gvk)
			if err != nil {
				return err
			}

			if isNamespaced {
				originalNamespace, err := getNamespace(node)
				if err != nil {
					return errors.Wrap(err, "failed to get namespace")
				}
				if originalNamespace == "" {
					return errors.New("failed to get namespace")
				}

				annotations, err := getAnnotations(node)
				if err != nil {
					return err
				}
				namespaces := map[string]struct{}{originalNamespace: {}}
				excludedNamespaces := map[string]struct{}{}
				namespacesAnnotation, ok := annotations[annotationNamespacesKey]
				if ok {
					for _, namespacesAnnotationNamespace := range strings.Split(namespacesAnnotation, ",") {
						if namespacesAnnotationNamespace == annotationNamespacesAll {
							for namespace := range allNamespaces {
								namespaces[namespace] = struct{}{}
							}
						} else if strings.HasPrefix(namespacesAnnotationNamespace, "-") {
							excludedNamespaces[strings.TrimPrefix(namespacesAnnotationNamespace, "-")] = struct{}{}
						} else {
							if _, ok := allNamespaces[namespacesAnnotationNamespace]; !ok {
								// We cannot allow this annotation to create new Namespaces because otherwise the meaning of "*" (annotationNamespacesAll) is inconsistent
								return fmt.Errorf("Namespace \"%s\" not found when processing annotation %s", namespacesAnnotationNamespace, annotationNamespacesKey)
							}
							namespaces[namespacesAnnotationNamespace] = struct{}{}
						}
					}
					// Clear annotation
					delete(annotations, annotationNamespacesKey)
					node.SetAnnotations(annotations)
				}

				for namespace := range namespaces {
					// Do not copy if namespace is excluded
					if _, ok := excludedNamespaces[namespace]; ok {
						continue
					}

					// Check if node matches another node once the namespace is modified. This allows `*` to
					// be used to specifiy a Namespace default but allow it to be overridden on a
					// per-Namespace basis
					nodeCopy := node.Copy()
					err = nodeCopy.SetNamespace(namespace)
					if err != nil {
						return err
					}
					if namespace != originalNamespace {
						clashing, err := o.isClashing(nodeCopy, yamlFileNodes, resourceInspector)
						if err != nil {
							return err
						}

						if clashing {
							continue
						}
					}

					newNodes = append(newNodes, nodeCopy)
				}
			} else {
				newNodes = append(newNodes, node)
			}
		}
		yamlFileNodes[yamlFile] = newNodes
	}
	return nil
}

func (o *Options) filterNodes(yamlFileNodes map[string][]*yaml.RNode) error {
	for yamlFile, nodes := range yamlFileNodes {
		filteredNodes := []*yaml.RNode{}
		for _, node := range nodes {
			isFiltered, err := o.isFiltered(node)
			if err != nil {
				return err
			}
			if isFiltered {
				continue
			}
			filteredNodes = append(filteredNodes, node)
		}
		yamlFileNodes[yamlFile] = filteredNodes
	}
	return nil
}

func (o *Options) defaultNamespaces(yamlFileNodes map[string][]*yaml.RNode, resourceInspector discovery.ResourceInspector) error {
	for yamlFile, nodes := range yamlFileNodes {
		newNodes := []*yaml.RNode{}
		for _, node := range nodes {

			namespace, err := getNamespace(node)
			if err != nil {
				return errors.Wrap(err, "failed to get namespace")
			}

			gvk, err := getGVK(node)
			if err != nil {
				return err
			}

			isNamespaced, err := resourceInspector.IsNamespaced(gvk)
			if err != nil {
				return err
			}

			if isNamespaced {
				if namespace == "" {
					if o.namespace != "" {
						namespace = o.namespace
					} else {
						namespace = corev1.NamespaceDefault
					}
					err = node.SetNamespace(namespace)
					if err != nil {
						return err
					}
				}
			} else {
				if namespace != "" {
					if o.clean {
						err = node.SetNamespace("")
						if err != nil {
							return err
						}
						namespace = ""
					}
				}

				if o.strict {
					if namespace != "" {
						return fmt.Errorf("metadata.namespace field should not be set for cluster-scoped resource: %s", gvk.String())
					}
				}
			}
			newNodes = append(newNodes, node)
		}
		yamlFileNodes[yamlFile] = newNodes
	}
	return nil
}

func (o *Options) getOutputFile(node *yaml.RNode, resourceInspector discovery.ResourceInspector) (string, error) {
	var outputFile string

	gvk, err := getGVK(node)
	if err != nil {
		return outputFile, err
	}

	isNamespaced, err := resourceInspector.IsNamespaced(gvk)
	if err != nil {
		return outputFile, err
	}

	name, err := getName(node)
	if err != nil {
		return outputFile, errors.Wrap(err, "failed to get name")
	}

	if isNamespaced {
		namespace, err := getNamespace(node)
		if err != nil {
			return outputFile, errors.Wrap(err, "failed to get namespace")
		}

		outputFile = o.getNamespacedOutputFile(name, namespace, gvk, resourceInspector)
	} else {
		outputFile = o.getNonNamespacedOutputFile(name, gvk, resourceInspector)
	}

	return outputFile, nil
}

func (o *Options) isClashing(candidateNode *yaml.RNode, yamlFileNodes map[string][]*yaml.RNode, resourceInspector discovery.ResourceInspector) (bool, error) {
	candidateOutputFile, err := o.getOutputFile(candidateNode, resourceInspector)
	if err != nil {
		return false, err
	}

	for _, nodes := range yamlFileNodes {
		for _, node := range nodes {
			outputFile, err := o.getOutputFile(node, resourceInspector)
			if err != nil {
				return false, err
			}

			if candidateOutputFile == outputFile {
				return true, nil
			}
		}
	}

	return false, nil
}

func (o *Options) isFiltered(node *yaml.RNode) (bool, error) {
	gvk, err := getGVK(node)
	if err != nil {
		return false, err
	}

	for _, filter := range o.filters {
		if gvk.GroupKind().String() == filter {
			return true, nil
		}
	}
	return false, nil
}

// findNamespaces finds used namespaces
func (o *Options) findNamespaces(nodes []*yaml.RNode, resourceInspector discovery.ResourceInspector) (map[string]struct{}, error) {
	namespaces := map[string]struct{}{}

	// Look for namespaces in each manifest
	for _, node := range nodes {

		kind, err := getKind(node)
		if err != nil {
			return namespaces, errors.Wrap(err, "failed to get kind")
		}

		if kind == "Namespace" {
			name, err := getName(node)
			if err != nil {
				return namespaces, err
			}

			namespaces[name] = struct{}{}
		} else {

			apiVersion, err := getAPIVersion(node)
			if err != nil {
				return namespaces, errors.Wrap(err, "failed to get apiVersion")
			}

			gvk := schema.FromAPIVersionAndKind(apiVersion, kind)

			// Ignore filtered resources
			isFiltered := false
			for _, filter := range o.filters {
				if gvk.GroupKind().String() == filter {
					isFiltered = true
					break
				}
			}
			if isFiltered {
				continue
			}

			isNamespaced, err := resourceInspector.IsNamespaced(gvk)
			if err != nil {
				return namespaces, err
			}

			if isNamespaced {
				namespace, err := getNamespace(node)
				if err != nil {
					return namespaces, err
				}
				if namespace == "" {
					if o.namespace != "" {
						namespace = o.namespace
					} else {
						namespace = corev1.NamespaceDefault
					}
				}
				namespaces[namespace] = struct{}{}
			}
		}
	}

	return namespaces, nil
}

// createMissingNamespaceManifests creates missing Namespace manifests
func (o *Options) createMissingNamespaceManifests(allNamespaces map[string]struct{}) error {
	if o.createMissingNamespaces {
		for namespace := range allNamespaces {
			namespaceFile := filepath.Join(o.output, nonNamespacedDirectory, "namespaces", namespace+".yaml")

			if _, err := os.Stat(namespaceFile); os.IsNotExist(err) {
				err = os.MkdirAll(filepath.Dir(namespaceFile), defaultDirectoryPerms)
				if err != nil {
					return err
				}

				namespaceManifest := fmt.Sprintf(manifestSeparator+`apiVersion: v1
kind: Namespace
metadata:
  name: %s
`, namespace)
				err = ioutil.WriteFile(namespaceFile, []byte(namespaceManifest), defaultFilePerms)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// listYAMLFiles lists YAML files to be processed
func listYAMLFiles(inputDir string) ([]string, error) {
	var files []string

	err := filepath.Walk(inputDir,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// Assume regular file is valid YAML file if it has an appropriate extension
			if info.Mode().IsRegular() && (strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml")) {
				files = append(files, path)
			}
			return nil
		})
	if err != nil {
		return files, err
	}

	return files, err
}

// findResources finds resources defined as CRDs to add to discovery
func findResources(nodes []*yaml.RNode) (map[schema.GroupVersionKind]bool, error) {
	resources := map[schema.GroupVersionKind]bool{}

	// Look for a resource definition in each manifest
	for _, node := range nodes {

		kind, err := getKind(node)
		if err != nil {
			return resources, err
		}

		if kind != "CustomResourceDefinition" {
			continue
		}

		resourceGroup, err := getCRDGroup(node)
		if err != nil {
			return resources, err
		}

		resourceKind, err := getCRDKind(node)
		if err != nil {
			return resources, err
		}

		resourceScope, err := getCRDScope(node)
		if err != nil {
			return resources, err
		}
		namespaced := false
		if resourceScope == "Namespaced" {
			namespaced = true
		}

		resourceVersions, err := getCRDVersions(node)
		if err != nil {
			return resources, err
		}

		for _, resourceVersion := range resourceVersions {
			gvk := schema.GroupVersionKind{
				Group:   resourceGroup,
				Version: resourceVersion,
				Kind:    resourceKind,
			}
			// TODO: should we allow discovery information to be overwritten?
			// if _, ok := resources[gvk]; ok {
			// 	return resources, fmt.Errorf("resource already exists: %s", gvk.String())
			// }
			resources[gvk] = namespaced
		}
	}

	return resources, nil
}

func (o *Options) writeNode(inputFile string, outputFile string, node *yaml.RNode) error {

	if !o.overwrite {
		// https://stackoverflow.com/a/12518877/6180803
		if _, err := os.Stat(outputFile); err == nil {
			return fmt.Errorf("file already exists: %s", outputFile)
		}
	}

	s, err := node.String()
	if err != nil {
		return err
	}

	err = os.MkdirAll(filepath.Dir(outputFile), defaultDirectoryPerms)
	if err != nil {
		return err
	}

	comment := ""
	if o.comment {
		comment = fmt.Sprintf("# Source: %s\n", inputFile)
	}
	err = ioutil.WriteFile(outputFile, []byte(manifestSeparator+comment+s), defaultFilePerms)
	if err != nil {
		return err
	}
	return nil
}

func (o *Options) getNonNamespacedOutputFile(name string, gvk schema.GroupVersionKind, resourceInspector discovery.ResourceInspector) string {
	subdirectory := pluralise(strings.ToLower(gvk.Kind))
	// Prefix with group if core
	if !resourceInspector.IsCoreGroup(gvk.Group) {
		subdirectory = pluralise(strings.ToLower(gvk.Kind)) + "." + gvk.Group
	}
	return filepath.Join(o.output, nonNamespacedDirectory, subdirectory, name+".yaml")
}

func (o *Options) getNamespacedOutputFile(name, namespace string, gvk schema.GroupVersionKind, resourceInspector discovery.ResourceInspector) string {
	fileName := strings.ToLower(gvk.Kind) + "-" + name + ".yaml"
	// Prefix with group if core
	if !resourceInspector.IsCoreGroup(gvk.Group) {
		fileName = strings.ToLower(gvk.Kind) + "." + gvk.Group + "-" + name + ".yaml"
	}

	return filepath.Join(o.output, namespacedDirectory, namespace, fileName)
}
