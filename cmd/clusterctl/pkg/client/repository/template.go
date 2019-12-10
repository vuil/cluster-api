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
	"bytes"
	"fmt"
	"strings"
	tmpl "text/template"

	"github.com/pkg/errors"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/config"
)

type TemplateOptions struct {
	ClusterName       string
	Namespace         string
	KubernetesVersion string
	ControlplaneCount int
	WorkerCount       int
}

// Template embeds a YAML file, read from the provider repository, defining the cluster templates (Cluster, Machines etc.).
// NB. Cluster templates are expected to exist for the infrastructure providers only
type Template interface {
	config.Provider
	Version() string
	Flavor() string
	Bootstrap() string
	Variables() []string
	TargetNamespace() string
	Yaml() []byte
}

// template implement Template
type template struct {
	config.Provider
	version         string
	flavor          string
	bootstrap       string
	variables       []string
	targetNamespace string
	yaml            []byte
}

var _ Template = &template{}

func (t *template) Version() string {
	return t.version
}

func (t *template) Flavor() string {
	return t.flavor
}

func (t *template) Bootstrap() string {
	return t.bootstrap
}

func (t *template) Variables() []string {
	return t.variables
}

func (t *template) TargetNamespace() string {
	return t.targetNamespace
}

func (t *template) Yaml() []byte {
	return t.yaml
}

// newTemplate returns a new objects embedding a cluster template YAML file
//
// It is important to notice that clusterctl applies a set of processing steps to the “raw” cluster template YAML read
// from the provider repositories:
// 1. Checks for all the variables in the cluster template YAML file and replace with corresponding config values
// 2. Process go templates contained in the cluster template YAML file
// 3. Ensure all the cluster objects are deployed in the target namespace
func newTemplate(provider config.Provider, version, flavor, bootstrap string, rawyaml []byte, configVariablesClient config.VariablesClient, targetNamespace string, options TemplateOptions) (*template, error) {

	// inspect the yaml for variables
	variables := inspectVariables(rawyaml)

	// Replace variables
	yaml, err := replaceVariables(rawyaml, variables, configVariablesClient)
	if err != nil {
		return nil, errors.Wrap(err, "failed to perform variable substitution")
	}

	// executes go templates
	yaml, err = execGoTemplates(yaml, options)
	if err != nil {
		return nil, err
	}

	// set target targetNamespace for all objects using kustomize
	yamlFile := "template.yaml"

	// gets a kustomization file for setting targetNamespace
	kustomization := getKustomizationFile(targetNamespace, yamlFile)

	// run kustomize build to get the final yaml
	finalYaml, err := kustomizeBuild(map[string][]byte{
		yamlFile:             yaml,
		"kustomization.yaml": []byte(kustomization),
	}, "")
	if err != nil {
		return nil, errors.Wrap(err, "failed to kustomize template namespace")
	}

	return &template{
		Provider:        provider,
		version:         version,
		flavor:          flavor,
		bootstrap:       bootstrap,
		variables:       variables,
		targetNamespace: targetNamespace,
		yaml:            finalYaml,
	}, nil
}

func execGoTemplates(yaml []byte, options TemplateOptions) ([]byte, error) {
	t := tmpl.New("cluster-template")

	t.Funcs(tmpl.FuncMap{
		"AdditionalCP": joinControlPlaneEnum,
	})

	t, err := t.Parse(string(yaml))
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	err = t.Execute(&buf, options)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func joinControlPlaneEnum(count int) []int {
	var i int
	var Items []int
	for i = 1; i < (count); i++ {
		Items = append(Items, i)
	}
	return Items
}

func getKustomizationFile(targetNamespace, fileName string) string {
	var sb strings.Builder
	sb.WriteString("apiVersion: kustomize.config.k8s.io/v1beta1\n")
	sb.WriteString("kind: Kustomization\n")

	sb.WriteString(fmt.Sprintf("namespace: \"%s\"\n", targetNamespace))

	sb.WriteString("resources:\n")
	sb.WriteString(fmt.Sprintf("- \"%s\"\n", fileName))

	return sb.String()
}
