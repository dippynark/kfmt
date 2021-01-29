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

	configSeparator = "---\n"

	nonNamespacedDirectory = "cluster"
	namespacedDirectory    = "namespaces"

	defaultFilePerms      = 0644
	defaultDirectoryPerms = 0755
)

var quotes = []string{"'", "\""}

type Options struct {
	output     string
	inputs     []string
	filters    []string
	namespace  string
	clean      bool
	strict     bool
	remove     bool
	comment    bool
	overwrite  bool
	discovery  bool
	kubeconfig string
}

func main() {
	o := &Options{}

	cmd := &cobra.Command{
		Use:   "kfmt",
		Short: "kfmt organises Kubernetes configs into a canonical format.",
		Run: func(cmd *cobra.Command, args []string) {
			err := o.Run()
			helper.CheckErr(err)
		},
	}

	cmd.Flags().BoolP("help", "h", false, "Help for kfmt")
	cmd.Flags().StringArrayVarP(&o.inputs, "input", "i", []string{}, "Input files or directories containing hydrated configs")
	cmd.Flags().StringVarP(&o.output, "output", "o", "", "Output directory to write structured configs")
	cmd.Flags().StringArrayVarP(&o.filters, "filter", "f", []string{}, "Filter kind.group from output configs (e.g. Deployment.apps or Secret)")
	cmd.Flags().StringVarP(&o.namespace, "namespace", "n", "", "Set namespace field if missing from namespaced resources")
	cmd.Flags().BoolVar(&o.clean, "clean", false, "Remove namespace field from non-namespaced resources")
	cmd.Flags().BoolVar(&o.strict, "strict", false, "Require namespace is not set for non-namespaced resources")
	cmd.Flags().BoolVar(&o.remove, "remove", false, "Remove processed input files")
	cmd.Flags().BoolVar(&o.comment, "comment", false, "Comment each output file with the relative path of corresponding input file")
	cmd.Flags().BoolVar(&o.overwrite, "overwrite", false, "Overwrite existing output files")
	cmd.Flags().BoolVar(&o.discovery, "discovery", false, "Use API Server for discovery")
	// https://github.com/kubernetes/client-go/blob/b72204b2445de5ac815ae2bb993f6182d271fdb4/examples/out-of-cluster-client-configuration/main.go#L45-L49
	if kubeconfigEnvVarValue := os.Getenv(kubeconfigEnvVar); kubeconfigEnvVarValue != "" {
		cmd.Flags().StringVarP(&o.kubeconfig, "kubeconfig", "k", kubeconfigEnvVarValue, "Absolute path to the kubeconfig file used for discovery")
	} else if home := homedir.HomeDir(); home != "" {
		cmd.Flags().StringVarP(&o.kubeconfig, "kubeconfig", "k", filepath.Join(home, ".kube", "config"), "Absolute path to the kubeconfig file used for discovery")
	} else {
		cmd.Flags().StringVarP(&o.kubeconfig, "kubeconfig", "k", "", "Absolute path to the kubeconfig file used for discovery")
	}

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func (o *Options) Run() error {

	if len(o.inputs) == 0 {
		return errors.New("no inputs specified")
	}
	if o.output == "" {
		return errors.Errorf("output directory not specified")
	}

	var yamlFiles []string
	for _, input := range o.inputs {
		info, err := os.Stat(input)
		if err != nil {
			return err
		}
		switch mode := info.Mode(); {
		case mode.IsDir():
			inputFiles, err := listYAMLFiles(input)
			if err != nil {
				return err
			}
			yamlFiles = append(yamlFiles, inputFiles...)
		case mode.IsRegular():
			yamlFiles = append(yamlFiles, input)
		default:
			return fmt.Errorf("%s is not a directory or regular file", input)
		}
	}

	// Discovery
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

	// Preprocess
	allNamespaces := map[string]struct{}{}
	for _, yamlFile := range yamlFiles {
		// Find local resources defined by CRDs
		resources, err := findResources(yamlFile)
		if err != nil {
			log.Fatalf("Failed to find CRDs in %s: %v", yamlFile, err)
		}
		for gvk, namespaced := range resources {
			resourceInspector.AddResource(gvk, namespaced)
		}

		// Find used Namespaces
		newNamespaces, err := o.findNamespaces(yamlFile, resourceInspector)
		if err != nil {
			log.Fatalf("Failed to find Namespaces in %s: %v", yamlFile, err)
		}
		for k, v := range newNamespaces {
			allNamespaces[k] = v
		}
	}

	// Move each YAML file into output directory structure
	for _, yamlFile := range yamlFiles {
		err := o.moveFile(yamlFile, resourceInspector, allNamespaces)
		if err != nil {
			return err
		}
	}

	// Create missing Namespace configs
	if err := o.createMissingNamespaces(allNamespaces); err != nil {
		return err
	}

	return nil
}

// findNamespaces finds used namespaces
func (o *Options) findNamespaces(inputFile string, resourceInspector discovery.ResourceInspector) (map[string]struct{}, error) {
	namespaces := map[string]struct{}{}

	b, err := ioutil.ReadFile(inputFile)
	if err != nil {
		return namespaces, err
	}

	nodes, err := kio.FromBytes(b)
	if err != nil {
		return namespaces, err
	}

	// Look for namespaces in each config
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
			for _, filter := range o.filters {
				if gvk.GroupKind().String() == filter {
					continue
				}
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
					namespace = o.namespace
				} else {
					// TODO: use default namespace from kubeconfig
					namespace = corev1.NamespaceDefault
				}
				namespaces[namespace] = struct{}{}
			}
		}
	}

	return namespaces, nil
}

// createMissingNamespaces creates missing Namespace configs
func (o *Options) createMissingNamespaces(allNamespaces map[string]struct{}) error {
	for namespace := range allNamespaces {
		namespaceFile := filepath.Join(o.output, nonNamespacedDirectory, "namespaces", namespace+".yaml")

		if _, err := os.Stat(namespaceFile); os.IsNotExist(err) {
			err = os.MkdirAll(filepath.Dir(namespaceFile), defaultDirectoryPerms)
			if err != nil {
				return err
			}

			namespaceConfig := fmt.Sprintf(configSeparator+`apiVersion: v1
kind: Namespace
metadata:
  name: %s
`, namespace)
			err = ioutil.WriteFile(namespaceFile, []byte(namespaceConfig), defaultFilePerms)
			if err != nil {
				return err
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
func findResources(inputFile string) (map[schema.GroupVersionKind]bool, error) {
	resources := map[schema.GroupVersionKind]bool{}

	b, err := ioutil.ReadFile(inputFile)
	if err != nil {
		return resources, err
	}

	nodes, err := kio.FromBytes(b)
	if err != nil {
		return resources, err
	}

	// Look for a resource definition in each config
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
			if _, ok := resources[gvk]; ok {
				return resources, fmt.Errorf("resource already exists: %s", gvk.String())
			}
			resources[gvk] = namespaced
		}
	}

	return resources, nil
}

// moveFile moves the input file into the right place in the output structure
func (o *Options) moveFile(inputFile string, resourceInspector discovery.ResourceInspector, allNamespaces map[string]struct{}) error {

	// Separate input file into individual configs
	b, err := ioutil.ReadFile(inputFile)
	if err != nil {
		return err
	}
	nodes, err := kio.FromBytes(b)
	if err != nil {
		return err
	}

	// Put each config into right location
	for _, node := range nodes {
		err = o.moveConfig(inputFile, node, resourceInspector, allNamespaces)
		if err != nil {
			return errors.Wrapf(err, "failed to process input file %s", inputFile)
		}
	}

	// Remove processed file
	if o.remove {
		err = os.Remove(inputFile)
		if err != nil {
			return errors.Wrapf(err, "failed to remove input file %s", inputFile)
		}
	}

	return nil
}

func (o *Options) moveConfig(inputFile string, node *yaml.RNode, resourceInspector discovery.ResourceInspector, allNamespaces map[string]struct{}) error {

	apiVersion, err := getAPIVersion(node)
	if err != nil {
		return errors.Wrap(err, "failed to get apiVersion")
	}

	kind, err := getKind(node)
	if err != nil {
		return errors.Wrap(err, "failed to get kind")
	}

	gvk := schema.FromAPIVersionAndKind(apiVersion, kind)

	// Ignore filtered resources
	for _, filter := range o.filters {
		if gvk.GroupKind().String() == filter {
			return nil
		}
	}

	namespace, err := getNamespace(node)
	if err != nil {
		return errors.Wrap(err, "failed to get namespace")
	}

	name, err := getName(node)
	if err != nil {
		return errors.Wrap(err, "failed to get name")
	}

	isNamespaced, err := resourceInspector.IsNamespaced(gvk)
	if err != nil {
		return err
	}

	isClusterScoped := !isNamespaced
	var outputFile string
	if isClusterScoped {
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
				return fmt.Errorf("namespace field should not be set for cluster-scoped resource: %s/%s", strings.ToLower(kind), name)
			}
		}

		outputFile = o.getNonNamespacedOutputFile(name, gvk, resourceInspector)
		err = o.writeNode(inputFile, outputFile, node)
		if err != nil {
			return err
		}
	} else {
		if namespace == "" {
			if o.namespace != "" {
				namespace = o.namespace
			} else {
				// TODO: use default namespace from kubeconfig
				namespace = corev1.NamespaceDefault
			}
			err = node.SetNamespace(namespace)
			if err != nil {
				return err
			}
		}

		annotations, err := getAnnotations(node)
		if err != nil {
			return err
		}
		namespaces := map[string]struct{}{namespace: {}}
		// TODO: remove annotation after processing
		namespacesAnnotation, ok := annotations[annotationNamespacesKey]
		if ok {
			if namespacesAnnotation == annotationNamespacesAll {
				namespaces = allNamespaces
			} else {
				for _, namespacesAnnotationNamespace := range strings.Split(namespacesAnnotation, ",") {
					if _, ok := allNamespaces[namespacesAnnotationNamespace]; !ok {
						// We cannot allow this annotation to create new Namespaces because otherwise the meaning of "*" (annotationNamespacesAll) is inconsistent
						return fmt.Errorf("Namespace \"%s\" not found when processing annotation %s", namespacesAnnotationNamespace, annotationNamespacesKey)
					}
					namespaces[namespacesAnnotationNamespace] = struct{}{}
				}
			}
		}

		for namespace := range namespaces {
			err = node.SetNamespace(namespace)
			if err != nil {
				return err
			}
			outputFile = o.getNamespacedOutputFile(name, namespace, gvk, resourceInspector)
			err = o.writeNode(inputFile, outputFile, node)
			if err != nil {
				return err
			}
		}
	}

	return nil
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

	relPath, err := filepath.Rel(filepath.Dir(outputFile), inputFile)
	if err != nil {
		return err
	}
	comment := ""
	if o.comment {
		comment = fmt.Sprintf("# Source: %s\n", relPath)
	}
	err = ioutil.WriteFile(outputFile, []byte(configSeparator+comment+s), defaultFilePerms)
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

func getAnnotations(node *yaml.RNode) (map[string]string, error) {
	annotations := map[string]string{}

	valueNode, err := node.Pipe(yaml.Lookup("metadata", "annotations"))
	if err != nil {
		return annotations, err
	}

	m := valueNode.Map()
	for k, v := range m {
		annotations[k] = v.(string)
	}

	return annotations, nil
}

func getNamespace(node *yaml.RNode) (string, error) {
	namespace, err := getStringField(node, "metadata", "namespace")
	if err != nil {
		return "", err
	}

	return namespace, nil
}

func getName(node *yaml.RNode) (string, error) {
	name, err := getStringField(node, "metadata", "name")
	if err != nil {
		return "", err
	}

	if name == "" {
		return "", errors.New("name is empty")
	}

	return name, nil
}

func getKind(node *yaml.RNode) (string, error) {
	kind, err := getStringField(node, "kind")
	if err != nil {
		return "", err
	}

	if kind == "" {
		return "", errors.New("kind is empty")
	}

	return kind, nil
}

func getAPIVersion(node *yaml.RNode) (string, error) {
	kind, err := getStringField(node, "apiVersion")
	if err != nil {
		return "", err
	}

	if kind == "" {
		return "", errors.New("apiVersion is empty")
	}

	return kind, nil
}

func getCRDGroup(node *yaml.RNode) (string, error) {
	group, err := getStringField(node, "spec", "group")
	if err != nil {
		return "", err
	}

	if group == "" {
		return "", errors.New("group is empty")
	}

	return group, nil
}

func getCRDKind(node *yaml.RNode) (string, error) {
	kind, err := getStringField(node, "spec", "names", "kind")
	if err != nil {
		return "", err
	}

	if kind == "" {
		return "", errors.New("CRD kind is empty")
	}

	return kind, nil
}

func getCRDScope(node *yaml.RNode) (string, error) {
	scope, err := getStringField(node, "spec", "scope")
	if err != nil {
		return "", err
	}

	if scope == "" {
		return "", errors.New("scope is empty")
	}

	return scope, nil
}

func getCRDVersions(node *yaml.RNode) ([]string, error) {
	valueNode, err := node.Pipe(yaml.Lookup("spec", "versions"))
	if err != nil {
		return nil, err
	}

	versions, err := valueNode.ElementValues("name")
	if err != nil {
		return nil, err
	}

	if len(versions) > 0 {
		return versions, nil
	}

	version, err := getStringField(node, "spec", "version")
	if err != nil {
		return nil, err
	}

	return []string{version}, nil
}

func getStringField(node *yaml.RNode, fields ...string) (string, error) {

	valueNode, err := node.Pipe(yaml.Lookup(fields...))
	if err != nil {
		return "", err
	}

	// Return empty string if value not found
	if valueNode == nil {
		return "", nil
	}

	value, err := valueNode.String()
	if err != nil {
		return "", nil
	}

	return trimSpaceAndQuotes(value), nil
}

// trimSpaceAndQuotes trims any whitespace and quotes around a value
func trimSpaceAndQuotes(value string) string {
	text := strings.TrimSpace(value)
	for _, q := range quotes {
		if strings.HasPrefix(text, q) && strings.HasSuffix(text, q) {
			return strings.TrimPrefix(strings.TrimSuffix(text, q), q)
		}
	}
	return text
}

func pluralise(lowercaseKind string) string {

	// e.g. ingress
	if strings.HasSuffix(lowercaseKind, "s") {
		return lowercaseKind + "es"
	}
	// e.g. networkpolicy
	if strings.HasSuffix(lowercaseKind, "cy") {
		return strings.TrimRight(lowercaseKind, "y") + "ies"
	}

	return lowercaseKind + "s"
}

func isWhitespaceOrComments(input string) bool {
	lines := strings.Split(input, "\n")
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t != "" && !strings.HasPrefix(t, "#") && !strings.HasPrefix(t, "--") {
			return false
		}
	}
	return true
}
