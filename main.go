package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/dippynark/config-cleaver/discovery"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/helper"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

const (
	inputDirFlag   = "input-dir"
	outputDirFlag  = "output-dir"
	discoveryFlag  = "discovery"
	kubeconfigFlag = "kubeconfig"

	kubeconfigEnvVar = "KUBECONFIG"

	configSeparator = "---\n"

	defaultFilePerms      = 0644
	defaultDirectoryPerms = 0755
)

var quotes = []string{"'", "\""}

type Options struct {
	kubeconfig string
	inputDirs  []string
	outputDir  string
	discovery  bool
}

func main() {
	o := &Options{}

	cmd := &cobra.Command{
		Use:   "kfmt",
		Short: "kfmt organises Kubernetes configs into a canonical structure.",
		Run: func(cmd *cobra.Command, args []string) {
			err := o.Run()
			helper.CheckErr(err)
		},
	}

	cmd.Flags().StringArrayVarP(&o.inputDirs, inputDirFlag, string([]rune(inputDirFlag)[0]), []string{}, "Directory containing hydrated configs")
	cmd.Flags().StringVarP(&o.outputDir, outputDirFlag, string([]rune(outputDirFlag)[0]), "", "Output directory")
	cmd.Flags().BoolVarP(&o.discovery, discoveryFlag, string([]rune(discoveryFlag)[0]), false, "Use API Server for discovery")

	// https://github.com/kubernetes/client-go/blob/b72204b2445de5ac815ae2bb993f6182d271fdb4/examples/out-of-cluster-client-configuration/main.go#L45-L49
	if kubeconfigEnvVarValue := os.Getenv(kubeconfigEnvVar); kubeconfigEnvVarValue != "" {
		cmd.Flags().StringVarP(&o.kubeconfig, kubeconfigFlag, string([]rune(kubeconfigFlag)[1]), kubeconfigEnvVarValue, "Absolute path to the kubeconfig file used for discovery")
	} else if home := homedir.HomeDir(); home != "" {
		cmd.Flags().StringVarP(&o.kubeconfig, kubeconfigFlag, string([]rune(kubeconfigFlag)[1]), filepath.Join(home, ".kube", "config"), "Absolute path to the kubeconfig file used for discovery")
	} else {
		cmd.Flags().StringVarP(&o.kubeconfig, kubeconfigFlag, string([]rune(kubeconfigFlag)[0]), "", "Absolute path to the kubeconfig file used for discovery")
	}

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func (o *Options) Run() error {

	if len(o.inputDirs) == 0 {
		return errors.Errorf("--%s is not set", inputDirFlag)
	}
	if o.outputDir == "" {
		return errors.Errorf("--%s is not set", outputDirFlag)
	}

	var yamlFiles []string
	for _, inputDir := range o.inputDirs {
		files, err := listYAMLFiles(inputDir)
		if err != nil {
			return err
		}
		yamlFiles = append(yamlFiles, files...)
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
			log.Fatalf("Failed to construct APIServer backed resource resource inspector: %v", err)
		}
	} else {
		resourceInspector = discovery.NewLocalResourceInspector()
	}

	// Find local resources defined by CRDs
	for _, yamlFile := range yamlFiles {
		resources, err := findResources(yamlFile)
		if err != nil {
			log.Fatalf("Failed to find CRDs in %s: %v", yamlFile, err)
		}
		for gvk, namespaced := range resources {
			resourceInspector.AddResource(gvk, namespaced)
		}
	}

	// Collect used namespaces
	var namespaces []string

	// Move each YAML file into output directory structure
	for _, yamlFile := range yamlFiles {
		err := moveFile(yamlFile, o.outputDir, resourceInspector, &namespaces)
		if err != nil {
			return err
		}
	}

	// Create missing Namespace configs
	if err := o.createMissingNamespaces(namespaces); err != nil {
		return err
	}

	return nil
}

// createMissingNamespaces creates missing Namespaces configs
func (o *Options) createMissingNamespaces(namespaces []string) error {
	for _, namespace := range namespaces {
		namespaceFile := filepath.Join(o.outputDir, "cluster", "namespaces", namespace+".yaml")

		if _, err := os.Stat(namespaceFile); os.IsNotExist(err) {
			err = os.MkdirAll(filepath.Dir(namespaceFile), defaultDirectoryPerms)
			if err != nil {
				return err
			}

			namespaceConfig := fmt.Sprintf(`apiVersion: v1
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

	// Separate input file into individual configs
	configs, err := splitFile(inputFile)
	if err != nil {
		return resources, err
	}

	// Look for a resource definition in each config
	for _, config := range configs {

		node, err := yaml.Parse(config)
		if err != nil {
			return resources, err
		}

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
func moveFile(inputFile, outputDir string, resourceInspector discovery.ResourceInspector, namespaces *[]string) error {

	// Separate input file into individual configs
	configs, err := splitFile(inputFile)
	if err != nil {
		return err
	}

	// Move each config into the right location
	for _, config := range configs {
		err = moveConfig(config, outputDir, resourceInspector, namespaces)
		if err != nil {
			return err
		}
	}

	// TODO: implement remove-input flag
	// rm inputFile

	return nil
}

func moveConfig(inputConfig, outputDir string, resourceInspector discovery.ResourceInspector, namespaces *[]string) error {

	node, err := yaml.Parse(inputConfig)
	if err != nil {
		return err
	}

	namespace, err := getNamespace(node)
	if err != nil {
		return errors.Wrap(err, "failed to get namespace")
	}

	name, err := getName(node)
	if err != nil {
		return errors.Wrap(err, "failed to get name")
	}

	kind, err := getKind(node)
	if err != nil {
		return errors.Wrap(err, "failed to get kind")
	}

	apiVersion, err := getAPIVersion(node)
	if err != nil {
		return errors.Wrap(err, "failed to get apiVersion")
	}

	// Generate destination file name
	gvk := schema.FromAPIVersionAndKind(apiVersion, kind)
	isNamespaced, err := resourceInspector.IsNamespaced(gvk)
	if err != nil {
		return err
	}

	isClusterScoped := !isNamespaced
	var outputFile string
	if isClusterScoped {
		if namespace != "" {
			return fmt.Errorf("namespace field should not be set for cluster-scoped resource: %s/%s", strings.ToLower(kind), name)
		}
		outputFile = filepath.Join(outputDir, "cluster", pluralise(strings.ToLower(kind)), name+".yaml")
	} else {
		if namespace == "" {
			// TODO: use default namespace from kubeconfig
			namespace = corev1.NamespaceDefault
		}
		*namespaces = append(*namespaces, namespace)
		outputFile = filepath.Join(outputDir, "namespaces", namespace, name+"-"+strings.ToLower(kind)+".yaml")
	}

	// Create destination directory
	err = os.MkdirAll(filepath.Dir(outputFile), defaultDirectoryPerms)
	if err != nil {
		return err
	}

	// Create destination file
	err = ioutil.WriteFile(outputFile, []byte(inputConfig), defaultFilePerms)
	if err != nil {
		return err
	}

	return nil
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
		return "", errors.New("kind is empty")
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

	if len(versions) == 0 {
		return nil, errors.New("no versions found")
	}

	return versions, nil
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

// splitFile splits a YAML file by the config separator
func splitFile(inputFile string) ([]string, error) {
	var configs []string

	data, err := ioutil.ReadFile(inputFile)
	if err != nil {
		return configs, err
	}
	inputData := string(data)

	// Avoid splitting if config separator is not at the start of the line
	if strings.HasPrefix(inputData, configSeparator) {
		inputData = "\n" + inputData
	}
	configStrings := strings.Split(inputData, "\n"+configSeparator)

	// Generate configs from configStrings
	for _, configString := range configStrings {
		// Ignore if config string is only whitespace or comments
		if isWhitespaceOrComments(configString) {
			continue
		}

		// Remove newline prefixes
		for {
			if !strings.HasPrefix(configString, "\n") {
				break
			}
			configString = strings.TrimPrefix(configString, "\n")
		}

		// Remove newline suffixes
		for {
			if !strings.HasSuffix(configString, "\n") {
				break
			}
			configString = strings.TrimSuffix(configString, "\n")
		}

		// Create buffer to build config
		buf := strings.Builder{}

		// Add separator at the top so that files can be `cat`ed together
		buf.WriteString(configSeparator)

		// Add sanitised config string
		buf.WriteString(configString)

		// Add trailing newline
		buf.WriteString("\n")

		configs = append(configs, buf.String())
	}

	return configs, nil
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
