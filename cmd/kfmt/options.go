package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dippynark/kfmt/pkg/discovery"
	"github.com/dippynark/kfmt/pkg/utils"
	"github.com/pkg/errors"
	"github.com/spf13/afero"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

type options struct {
	output                  string
	inputs                  []string
	filters                 []string
	gvkScopes               []string
	namespace               string
	clean                   bool
	strict                  bool
	remove                  bool
	comment                 bool
	overwrite               bool
	createMissingNamespaces bool
	discovery               bool
	kubeconfig              string
	version                 bool
}

func (o *options) run() error {

	if o.version {
		fmt.Println(version)
		return nil
	}

	err := o.validate()
	if err != nil {
		return err
	}

	// Abstract away from the filesystem: https://github.com/spf13/afero
	return o.format(afero.NewOsFs(), afero.NewBasePathFs(afero.NewOsFs(), o.output))
}

func (o *options) validate() error {
	if o.output == "" {
		return errors.Errorf("output directory not specified")
	}
	return nil
}

func (o *options) format(inputFileSystem afero.Fs, outputFileSystem afero.Fs) error {
	// Initialise discovery to determine whether resources are namespaced or not
	resourceInspector, err := o.getResourceInspector()
	if err != nil {
		return err
	}

	// Find all YAML files specified as input
	yamlFiles, err := o.findYAMLFiles(inputFileSystem)
	if err != nil {
		return err
	}

	// Map input files to nodes (parsed YAML documents)
	yamlFileNodes, err := o.findYAMLFileNodes(inputFileSystem, yamlFiles)
	if err != nil {
		return err
	}

	// Add local CRDs to discovery
	err = o.localDiscovery(yamlFileNodes, resourceInspector)
	if err != nil {
		return err
	}

	// Add manually specified GVK scopes to discovery
	err = o.manualDiscovery(resourceInspector)
	if err != nil {
		return err
	}

	// Find all Namespaces either declared as resources or appearing in the metadata.namespace field
	allNamespaces, err := o.findAllNamespaces(yamlFileNodes, resourceInspector)
	if err != nil {
		return err
	}

	// Remove nodes that match filters
	err = o.filterNodes(yamlFileNodes)
	if err != nil {
		return err
	}

	// Apply Namespace field defaults to namespaced resources and remove Namespace field from
	// non-namespaced resources
	err = o.defaultNamespaces(yamlFileNodes, resourceInspector)
	if err != nil {
		return err
	}

	// Apply `kfmt.dev/namespaces` annotation
	err = o.mirrorNodes(yamlFileNodes, allNamespaces, resourceInspector)
	if err != nil {
		return err
	}

	// Write nodes to disk into output directory
	err = o.writeManifests(yamlFileNodes, resourceInspector, outputFileSystem)
	if err != nil {
		return err
	}

	// Remove processed YAML files
	err = o.removeYAMLFiles(yamlFileNodes, outputFileSystem)
	if err != nil {
		return err
	}

	// Create missing Namespace manifests
	if err := o.createMissingNamespaceManifests(allNamespaces, outputFileSystem); err != nil {
		return err
	}

	return nil
}

func (o *options) getResourceInspector() (discovery.ResourceInspector, error) {
	var resourceInspector discovery.ResourceInspector
	if o.discovery {
		restcfg, err := clientcmd.BuildConfigFromFlags("", o.kubeconfig)
		if err != nil {
			return resourceInspector, errors.Wrap(err, "failed to build kubernetes REST client config")
		}
		resourceInspector, err = discovery.NewAPIServerResourceInspector(restcfg)
		if err != nil {
			return resourceInspector, errors.Wrap(err, "failed to construct APIServer backed resource inspector")
		}
	} else {
		var err error
		resourceInspector, err = discovery.NewLocalResourceInspector()
		if err != nil {
			return resourceInspector, errors.Wrap(err, "failed to construct locally backed resource inspector")
		}
	}

	return resourceInspector, nil
}

func (o *options) findYAMLFiles(inputFileSystem afero.Fs) ([]string, error) {
	var yamlFiles []string
	if len(o.inputs) > 0 {
		for _, input := range o.inputs {
			info, err := inputFileSystem.Stat(input)
			if err != nil {
				return yamlFiles, err
			}
			switch mode := info.Mode(); {
			case mode.IsDir():
				inputFiles, err := listYAMLFiles(inputFileSystem, input)
				if err != nil {
					return yamlFiles, err
				}
				yamlFiles = append(yamlFiles, inputFiles...)
			default:
				yamlFiles = append(yamlFiles, input)
			}
		}
	} else {
		// Read from stdin if no input specified
		yamlFiles = []string{os.Stdin.Name()}
	}
	return yamlFiles, nil
}

func (o *options) findYAMLFileNodes(inputFileSystem afero.Fs, yamlFiles []string) (map[string][]*yaml.RNode, error) {
	yamlFileNodes := map[string][]*yaml.RNode{}
	for _, yamlFile := range yamlFiles {
		b, err := afero.ReadFile(inputFileSystem, yamlFile)
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

func (o *options) localDiscovery(yamlFileNodes map[string][]*yaml.RNode, resourceInspector discovery.ResourceInspector) error {
	for yamlFile, nodes := range yamlFileNodes {
		resources, err := findResources(nodes)
		if err != nil {
			return errors.Wrapf(err, "failed to find CRDs in %s", yamlFile)
		}
		for gvk, namespaced := range resources {
			resourceInspector.AddGVKToScope(gvk, namespaced)
		}
	}
	return nil
}

func (o *options) manualDiscovery(resourceInspector discovery.ResourceInspector) error {
	for _, gvkScope := range o.gvkScopes {
		i := strings.Index(gvkScope, ":")
		if i == -1 {
			return fmt.Errorf("failed to parse GVK scope: %s", gvkScope)
		}

		gvkString := gvkScope[:i]
		scope := gvkScope[i+1:]

		var namespaced bool
		switch apiextensions.ResourceScope(scope) {
		case apiextensions.ClusterScoped:
			namespaced = false
		case apiextensions.NamespaceScoped:
			namespaced = true
		default:
			return fmt.Errorf("unrecognised scope %s", scope)
		}

		i = strings.Index(gvkString, ".")
		if i == -1 {
			return fmt.Errorf("failed to parse GVK: %s", gvkString)
		}

		kind := gvkString[:i]
		gvString := gvkString[i+1:]

		gv, err := schema.ParseGroupVersion(gvString)
		if err != nil {
			return err
		}

		gvk := schema.GroupVersionKind{
			Group:   gv.Group,
			Version: gv.Version,
			Kind:    kind,
		}

		resourceInspector.AddGVKToScope(gvk, namespaced)
	}

	return nil
}

func (o *options) findAllNamespaces(yamlFileNodes map[string][]*yaml.RNode, resourceInspector discovery.ResourceInspector) (map[string]struct{}, error) {
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

func (o *options) filterNodes(yamlFileNodes map[string][]*yaml.RNode) error {
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

func (o *options) defaultNamespaces(yamlFileNodes map[string][]*yaml.RNode, resourceInspector discovery.ResourceInspector) error {
	for yamlFile, nodes := range yamlFileNodes {
		newNodes := []*yaml.RNode{}
		for _, node := range nodes {

			namespace, err := utils.GetNamespace(node)
			if err != nil {
				return errors.Wrap(err, "failed to get namespace")
			}

			gvk, err := utils.GetGVK(node)
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

func (o *options) mirrorNodes(yamlFileNodes map[string][]*yaml.RNode, allNamespaces map[string]struct{}, resourceInspector discovery.ResourceInspector) error {
	for yamlFile, nodes := range yamlFileNodes {
		newNodes := []*yaml.RNode{}
		for _, node := range nodes {

			gvk, err := utils.GetGVK(node)
			if err != nil {
				return err
			}

			isNamespaced, err := resourceInspector.IsNamespaced(gvk)
			if err != nil {
				return err
			}

			if isNamespaced {
				originalNamespace, err := utils.GetNamespace(node)
				if err != nil {
					return errors.Wrap(err, "failed to get namespace")
				}
				if originalNamespace == "" {
					return errors.New("failed to get namespace")
				}

				annotations, err := utils.GetAnnotations(node)
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

func (o *options) writeManifests(yamlFileNodes map[string][]*yaml.RNode, resourceInspector discovery.ResourceInspector, outputFilesystem afero.Fs) error {
	for yamlFile, nodes := range yamlFileNodes {
		for _, node := range nodes {

			outputFile, err := o.getOutputFile(node, resourceInspector)
			if err != nil {
				return err
			}

			err = o.writeManifest(yamlFile, outputFile, node, outputFilesystem)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (o *options) removeYAMLFiles(yamlFileNodes map[string][]*yaml.RNode, outputFilesystem afero.Fs) error {
	if o.remove {
		for yamlFile := range yamlFileNodes {
			// Ignore stdin
			if yamlFile == os.Stdin.Name() {
				continue
			}
			err := outputFilesystem.Remove(yamlFile)
			if err != nil {
				return errors.Wrapf(err, "failed to remove input file %s", yamlFile)
			}
		}
	}
	return nil
}

// createMissingNamespaceManifests creates missing Namespace manifests
func (o *options) createMissingNamespaceManifests(allNamespaces map[string]struct{}, outputFilesystem afero.Fs) error {
	if o.createMissingNamespaces {
		for namespace := range allNamespaces {
			namespaceFile := filepath.Join(o.output, nonNamespacedDirectory, "namespaces", namespace+".yaml")

			if _, err := outputFilesystem.Stat(namespaceFile); os.IsNotExist(err) {
				err = outputFilesystem.MkdirAll(filepath.Dir(namespaceFile), defaultDirectoryPerms)
				if err != nil {
					return err
				}

				namespaceManifest := fmt.Sprintf(manifestSeparator+`apiVersion: v1
kind: Namespace
metadata:
  name: %s
`, namespace)
				err = afero.WriteFile(outputFilesystem, namespaceFile, []byte(namespaceManifest), defaultFilePerms)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (o *options) getOutputFile(node *yaml.RNode, resourceInspector discovery.ResourceInspector) (string, error) {
	var outputFile string

	gvk, err := utils.GetGVK(node)
	if err != nil {
		return outputFile, err
	}

	isNamespaced, err := resourceInspector.IsNamespaced(gvk)
	if err != nil {
		return outputFile, err
	}

	name, err := utils.GetName(node)
	if err != nil {
		return outputFile, errors.Wrap(err, "failed to get name")
	}

	if isNamespaced {
		namespace, err := utils.GetNamespace(node)
		if err != nil || namespace == "" {
			return outputFile, errors.Wrap(err, "failed to get namespace")
		}

		outputFile = o.getNamespacedOutputFile(name, namespace, gvk, resourceInspector)
	} else {
		outputFile = o.getNonNamespacedOutputFile(name, gvk, resourceInspector)
	}

	return outputFile, nil
}

func (o *options) isClashing(candidateNode *yaml.RNode, yamlFileNodes map[string][]*yaml.RNode, resourceInspector discovery.ResourceInspector) (bool, error) {
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

func (o *options) isFiltered(node *yaml.RNode) (bool, error) {
	gvk, err := utils.GetGVK(node)
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
func (o *options) findNamespaces(nodes []*yaml.RNode, resourceInspector discovery.ResourceInspector) (map[string]struct{}, error) {
	namespaces := map[string]struct{}{}

	// Look for namespaces in each manifest
	for _, node := range nodes {

		kind, err := utils.GetKind(node)
		if err != nil {
			return namespaces, errors.Wrap(err, "failed to get kind")
		}

		if kind == "Namespace" {
			name, err := utils.GetName(node)
			if err != nil {
				return namespaces, err
			}

			namespaces[name] = struct{}{}
		} else {

			apiVersion, err := utils.GetAPIVersion(node)
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
				namespace, err := utils.GetNamespace(node)
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

// listYAMLFiles lists YAML files to be processed
func listYAMLFiles(inputFileSystem afero.Fs, inputDir string) ([]string, error) {
	var files []string

	err := afero.Walk(inputFileSystem, inputDir,
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

		kind, err := utils.GetKind(node)
		if err != nil {
			return resources, err
		}

		if kind != "CustomResourceDefinition" {
			continue
		}

		resourceGroup, err := utils.GetCRDGroup(node)
		if err != nil {
			return resources, err
		}

		resourceKind, err := utils.GetCRDKind(node)
		if err != nil {
			return resources, err
		}

		resourceScope, err := utils.GetCRDScope(node)
		if err != nil {
			return resources, err
		}
		namespaced := false
		if resourceScope == "Namespaced" {
			namespaced = true
		}

		resourceVersions, err := utils.GetCRDVersions(node)
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

func (o *options) writeManifest(inputFile string, outputFile string, node *yaml.RNode, outputFilesystem afero.Fs) error {

	if !o.overwrite {
		// https://stackoverflow.com/a/12518877/6180803
		if _, err := outputFilesystem.Stat(outputFile); err == nil {
			return fmt.Errorf("file already exists: %s", outputFile)
		}
	}

	s, err := node.String()
	if err != nil {
		return err
	}

	err = outputFilesystem.MkdirAll(filepath.Dir(outputFile), defaultDirectoryPerms)
	if err != nil {
		return err
	}

	comment := ""
	if o.comment {
		comment = fmt.Sprintf("# Source: %s\n", inputFile)
	}
	err = afero.WriteFile(outputFilesystem, outputFile, []byte(manifestSeparator+comment+s), defaultFilePerms)
	if err != nil {
		return err
	}
	return nil
}

func (o *options) getNonNamespacedOutputFile(name string, gvk schema.GroupVersionKind, resourceInspector discovery.ResourceInspector) string {
	subdirectory := utils.Pluralise(strings.ToLower(gvk.Kind))
	// Prefix with group if not core
	if !resourceInspector.IsCoreGroup(gvk.Group) {
		subdirectory = utils.Pluralise(strings.ToLower(gvk.Kind)) + "." + gvk.Group
	}
	return filepath.Join(o.output, nonNamespacedDirectory, subdirectory, name+".yaml")
}

func (o *options) getNamespacedOutputFile(name, namespace string, gvk schema.GroupVersionKind, resourceInspector discovery.ResourceInspector) string {
	fileName := strings.ToLower(gvk.Kind) + "-" + name + ".yaml"
	// Prefix with group if not core
	if !resourceInspector.IsCoreGroup(gvk.Group) {
		fileName = strings.ToLower(gvk.Kind) + "." + gvk.Group + "-" + name + ".yaml"
	}

	return filepath.Join(o.output, namespacedDirectory, namespace, fileName)
}
