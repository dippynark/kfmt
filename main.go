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
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

const (
	inputDirFlag  = "input-dir"
	outputDirFlag = "output-dir"
	discoveryFlag = "discovery"

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
		Use:   "cleave",
		Short: "Cleaves configs into gitops structure",
		Run: func(cmd *cobra.Command, args []string) {
			err := o.Run()
			helper.CheckErr(err)
		},
	}
	cmd.Flags().StringVar(&o.kubeconfig, "kubeconfig", "", "Path to a KUBECONFIG file used to lookup discovery information")
	cmd.Flags().StringArrayVarP(&o.inputDirs, inputDirFlag, string([]rune(inputDirFlag)[1]), []string{}, "the directory containing hydrated configs")
	cmd.Flags().StringVarP(&o.outputDir, outputDirFlag, string([]rune(outputDirFlag)[1]), "", "the output directory")
	cmd.Flags().BoolVarP(&o.discovery, discoveryFlag, string([]rune(discoveryFlag)[1]), false, "use discovery")

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

	// Discovery
	var inspector discovery.ResourceInspector
	if o.discovery {
		restcfg, err := clientcmd.BuildConfigFromFlags("", o.kubeconfig)
		if err != nil {
			log.Fatalf("Failed to build kubernetes REST client config: %v", err)
		}
		inspector, err = discovery.NewAPIServerResourceInspector(restcfg)
		if err != nil {
			log.Fatalf("Failed to construct APIServer backed resource inspector: %v", err)
		}
	} else {
		var err error
		inspector, err = discovery.NewLocalResourceInspector()
		if err != nil {
			log.Fatalf("Failed to construct local resource inspector: %v", err)
		}
	}

	var yamlFiles []string
	for _, inputDir := range o.inputDirs {
		files, err := listYAMLFiles(inputDir)
		if err != nil {
			return err
		}
		yamlFiles = append(yamlFiles, files...)
	}

	// Move each YAML file into output directory structure
	for _, yamlFile := range yamlFiles {
		err := moveFile(yamlFile, o.outputDir, inspector)
		if err != nil {
			return err
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

// moveFile moves the input file into the right place in the output structure
func moveFile(inputFile, outputDir string, inspector discovery.ResourceInspector) error {

	// Separate input file into individual configs
	configs, err := splitFile(inputFile)
	if err != nil {
		return err
	}

	// Move each config into the right location
	for _, config := range configs {
		err = moveConfig(config, outputDir, inspector)
		if err != nil {
			return err
		}
	}

	// TODO: implement remove-input flag
	// rm inputFile

	return nil
}

func moveConfig(inputConfig, outputDir string, inspector discovery.ResourceInspector) error {

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
	isNamespaced, err := inspector.IsNamespaced(gvk)
	if err != nil {
		return err
	}
	/*isClusterScoped := strings.HasPrefix(kind, "Cluster") ||
	(kind == "CustomResourceDefinition") ||
	(kind == "Namespace")*/
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

	// Create missing namespace
	if !isClusterScoped {
		namespaceFile := filepath.Join(outputDir, "cluster", "namespaces", namespace+".yaml")
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
