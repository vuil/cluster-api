/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package repository

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/scheme"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/util"
	"sigs.k8s.io/yaml"
)

type Components interface {
	config.Provider
	Version() string
	Variables() []string
	TargetNamespace() string
	WatchingNamespace() string
	Metadata() clusterctlv1.Provider
	Objs() []unstructured.Unstructured
	Yaml() ([]byte, error)
}

type components struct {
	config.Provider
	version           string
	variables         []string
	targetNamespace   string
	watchingNamespace string
	objs              []unstructured.Unstructured
}

var _ Components = &components{}

func (c *components) Version() string {
	return c.version
}

func (c *components) Variables() []string {
	return c.variables
}

func (c *components) TargetNamespace() string {
	return c.targetNamespace
}

func (c *components) WatchingNamespace() string {
	return c.watchingNamespace
}

func (c *components) Metadata() clusterctlv1.Provider {
	return clusterctlv1.Provider{
		TypeMeta: metav1.TypeMeta{
			APIVersion: clusterctlv1.GroupVersion.String(),
			Kind:       "Provider",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: c.targetNamespace,
			Name:      c.Name(),
			Labels:    getLabels(c.Name()),
		},
		Type:             string(c.Type()),
		Version:          c.version,
		WatchedNamespace: c.watchingNamespace,
	}
}

func (c *components) Objs() []unstructured.Unstructured {
	return c.objs
}

func (c *components) Yaml() ([]byte, error) {
	var ret [][]byte //nolint
	for _, o := range c.objs {
		content, err := yaml.Marshal(o)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to marshal yaml for %s/%s", o.GetNamespace(), o.GetName())
		}
		ret = append(ret, content)
	}

	return util.JoinYaml(ret...), nil
}

func newComponents(provider config.Provider, version string, files map[string][]byte, kustomizeDir string, configVariablesClient config.VariablesClient, targetNamespace, watchingNamespace string) (*components, error) {

	// use kustomize build to generate 1 single yaml file for the component manifest, if required
	rawyaml, err := kustomizeBuild(files, kustomizeDir)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate the component manifest")
	}

	// inspect the yaml read from the repository for variables
	variables := inspectVariables(rawyaml)

	// Replace variables
	yaml, err := replaceVariables(rawyaml, variables, configVariablesClient)
	if err != nil {
		return nil, errors.Wrap(err, "failed to perform variable substitution")
	}

	// transform the yaml in a list of objects
	objs, err := util.ToUnstructured(yaml)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse yaml")
	}

	// inspect the list of objects for the default target namespace
	// the default target namespace is the namespace object defined in the component yaml read from the repository, if any
	defaultTargetNamespace, err := inspectTargetNamespace(objs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to detect default target namespace")
	}

	// if target namespaces is empty defaultTargetNamespace is used. In case also defaultTargetNamespace
	// is empty, an error is returned
	if targetNamespace == "" {
		targetNamespace = defaultTargetNamespace
	}

	if targetNamespace == "" {
		return nil, errors.New("target namespace can't be defaulted. Please specify a target namespace")
	}

	// inspect the list of objects for the default watching namespace
	// the default watching namespace is the namespace the controller is set for watching in the component yaml read from the repository, if any
	defaultWatchingNamespace, err := inspectWatchNamespace(objs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to detect default watching namespace")
	}

	objs = fixTargetNamespace(objs, targetNamespace)

	if defaultWatchingNamespace != watchingNamespace {
		objs, err = fixWatchNamespace(objs, watchingNamespace)
		if err != nil {
			return nil, errors.Wrap(err, "failed to set watching namespace")
		}
	}

	objs = addLabels(objs, provider.Name())

	return &components{
		Provider:          provider,
		version:           version,
		variables:         variables,
		targetNamespace:   targetNamespace,
		watchingNamespace: watchingNamespace,
		objs:              objs,
	}, nil
}

func getLabels(name string) map[string]string {
	return map[string]string{
		clusterctlv1.ClusterctlLabelName:         "",
		clusterctlv1.ClusterctlProviderLabelName: name,
	}
}

func kustomizeBuild(files map[string][]byte, kustomizeDir string) ([]byte, error) {
	if len(files) == 0 {
		return nil, nil
	}

	if len(files) == 1 {
		// TODO: find a more idiomatic way to get the first (the only) element in a map
		for _, v := range files {
			return v, nil
		}
	}

	dir, err := ioutil.TempDir("", "clusterctl")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a temp directory for generating component manifest")
	}
	defer os.RemoveAll(dir)

	nested := false
	for f, c := range files {
		filePath := filepath.Join(dir, f)
		if filepath.Dir(filePath) != dir {
			nested = true
			if err := os.MkdirAll(filepath.Dir(filePath), 0700); err != nil {
				return nil, errors.Wrapf(err, "failed to create tmp dir for  %s component manifest file", f)
			}
		}
		if err := ioutil.WriteFile(filePath, c, 0640); err != nil {
			return nil, errors.Wrapf(err, "failed to write tmp file for  %s component manifest file", f)
		}
	}

	kustomizeDir = filepath.Join(dir, kustomizeDir)

	// Build the manifest by running kubectl kustomize in case all the resources are a single, flat folder (this
	// is usually the case when working with assets that are part of a release).

	var out string
	if !nested {
		lines, err := util.NewCmd("kubectl", "kustomize", kustomizeDir).RunAndCapture()
		if err != nil {
			return nil, errors.Wrap(err, "failed to build the manifest file using kubectl kustomize")
		}
		out = strings.Join(lines, "\n")
		return []byte(out), nil
	}

	// Instead, in case the resources are in nested folder like e.g. when deploying from a /config source folder, we are forced to
	// use kustomize because the version of kustomize embedded in kubectl is quite older.
	// This should not be a problem, because in this case the user most probably already has kustomize installed.

	lines, err := util.NewCmd("kustomize", "build", kustomizeDir).RunAndCapture()
	if err != nil {
		return nil, errors.Wrap(err, "failed to build the manifest file using kustomize build")
	}
	out = strings.Join(lines, "\n")
	return []byte(out), nil
}

var variableRegEx = regexp.MustCompile(`\${\s*([A-Z0-9_]+)\s*}`)

func inspectVariables(data []byte) []string {
	variables := map[string]string{}
	match := variableRegEx.FindAllStringSubmatch(string(data), -1)

	for _, m := range match {
		submatch := m[1]
		if _, ok := variables[submatch]; !ok {
			variables[submatch] = ""
		}
	}

	var ret []string // nolint
	for v := range variables {
		ret = append(ret, v)
	}

	sort.Strings(ret)

	return ret
}

func replaceVariables(yaml []byte, variables []string, configVariablesClient config.VariablesClient) ([]byte, error) {
	tmp := string(yaml)
	var missingVariables []string
	for _, key := range variables {
		val, err := configVariablesClient.Get(key)
		if err != nil {
			missingVariables = append(missingVariables, key)
			continue
		}
		exp := regexp.MustCompile(`\$\{\s*` + key + `\s*\}`)
		tmp = exp.ReplaceAllString(tmp, val)
	}
	if len(missingVariables) > 0 {
		return nil, errors.Errorf("value for variables [%s] is not set. Please set the value using os environment variables or the clusterctl config file", strings.Join(missingVariables, ", "))
	}

	return []byte(tmp), nil
}

const namespaceKind = "Namespace"

func inspectTargetNamespace(objs []unstructured.Unstructured) (string, error) {
	namespace := ""
	for _, o := range objs {
		// if the object has Kind Namespace
		if o.GetKind() == namespaceKind {
			// grab the name (or error if there is more than one Namespace object)
			if namespace != "" {
				return "", errors.New("Invalid manifest. There should be no more than one resource with Kind Namespace in the provider components yaml")
			}
			namespace = o.GetName()
		}
	}
	return namespace, nil
}

func fixTargetNamespace(objs []unstructured.Unstructured, targetNamespace string) []unstructured.Unstructured {
	namespaceObjectFound := false

	for _, o := range objs {
		// if the object has Kind Namespace, fix the namespace name
		if o.GetKind() == namespaceKind {
			namespaceObjectFound = true
			o.SetName(targetNamespace)
		}

		// if the object is namespaced, set the namespace name
		if isResourceNamespaced(o.GetKind()) {
			o.SetNamespace(targetNamespace)
		}
	}

	// if there isn't an object with Kind Namespace, add it
	if !namespaceObjectFound {
		objs = append(objs, unstructured.Unstructured{
			Object: map[string]interface{}{
				"kind": namespaceKind,
				"metadata": map[string]interface{}{
					"name": targetNamespace,
				},
			},
		})
	}

	return objs
}

//TODO: move in utils
func isResourceNamespaced(kind string) bool {
	switch kind {
	case "Namespace",
		"Node",
		"PersistentVolume",
		"PodSecurityPolicy",
		"CertificateSigningRequest",
		"ClusterRoleBinding",
		"ClusterRole",
		"VolumeAttachment",
		"StorageClass",
		"CSIDriver",
		"CSINode",
		"ValidatingWebhookConfiguration",
		"MutatingWebhookConfiguration",
		"CustomResourceDefinition",
		"PriorityClass",
		"RuntimeClass":
		return false
	default:
		return true
	}
}

const namespaceArgPrefix = "--namespace="
const deploymentKind = "Deployment"
const controllerContainerName = "manager"

func inspectWatchNamespace(objs []unstructured.Unstructured) (string, error) {
	namespace := ""
	// look for resources of kind Deployment
	for _, o := range objs {
		if o.GetKind() == deploymentKind {

			// Convert Unstructured into a typed object
			d := &appsv1.Deployment{}
			if err := scheme.Scheme.Convert(&o, d, nil); err != nil { //nolint
				return "", err
			}

			// look for a container with name "manager"
			for _, c := range d.Spec.Template.Spec.Containers {
				if c.Name == controllerContainerName {

					// look for the --namespace command arg
					for _, a := range c.Args {
						if strings.HasPrefix(a, namespaceArgPrefix) {
							n := strings.TrimPrefix(a, namespaceArgPrefix)
							if namespace != "" && n != namespace {
								return "", errors.New("Invalid manifest. All the controllers should watch have the same --namespace command arg in the provider components yaml")
							}
							namespace = n
						}
					}
				}
			}
		}
	}

	return namespace, nil
}

func fixWatchNamespace(objs []unstructured.Unstructured, watchingNamespace string) ([]unstructured.Unstructured, error) {

	// look for resources of kind Deployment
	for i, o := range objs {
		if o.GetKind() == deploymentKind {

			// Convert Unstructured into a typed object
			d := &appsv1.Deployment{}
			if err := scheme.Scheme.Convert(&o, d, nil); err != nil { //nolint
				return nil, err
			}

			// look for a container with name "manager"
			for j, c := range d.Spec.Template.Spec.Containers {
				if c.Name == controllerContainerName {

					// look for the --namespace command arg
					found := false
					for k, a := range c.Args {
						// if it exist
						if strings.HasPrefix(a, namespaceArgPrefix) {
							found = true

							// replace the command arg with the desired value or delete the arg if the controller should watch for objects in all the namespaces
							if watchingNamespace != "" {
								c.Args[k] = fmt.Sprintf("%s%s", namespaceArgPrefix, watchingNamespace)
								continue
							}
							c.Args = remove(c.Args, k)
						}
					}

					// if it not exists, and the controller should watch for objects in a specific namespace, set the command arg
					if !found && watchingNamespace != "" {
						c.Args = append(c.Args, fmt.Sprintf("%s%s", namespaceArgPrefix, watchingNamespace))
					}
				}

				d.Spec.Template.Spec.Containers[j] = c
			}

			// Convert Deployment back to Unstructured
			if err := scheme.Scheme.Convert(d, &o, nil); err != nil { //nolint
				return nil, err
			}
			objs[i] = o
		}
	}
	return objs, nil
}

func remove(slice []string, i int) []string {
	copy(slice[i:], slice[i+1:])
	return slice[:len(slice)-1]
}

func addLabels(objs []unstructured.Unstructured, name string) []unstructured.Unstructured {
	for _, o := range objs {
		labels := o.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}
		for k, v := range getLabels(name) {
			labels[k] = v
		}
		o.SetLabels(labels)
	}

	return objs
}
