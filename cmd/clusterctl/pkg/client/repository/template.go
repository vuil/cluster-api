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

type Template interface {
	config.Provider
	Version() string
	Flavor() string
	Bootstrap() string
	Path() string
	Variables() []string
	TargetNamespace() string
	Yaml() []byte
}

type template struct {
	config.Provider
	version         string
	flavor          string
	bootstrap       string
	path            string
	variables       []string
	rawyaml         []byte
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

func (t *template) Path() string {
	return t.path
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

// templateBuilder provides an helper struct used to split
// the sequence of steps required to get config templates ready to be installed
// into smaller, easily manageable pieces.
type templateBuilder struct {
	template              *template
	configVariablesClient config.VariablesClient
}

func newTemplateBuilder(provider config.Provider, configVariablesClient config.VariablesClient) templateBuilder {
	return templateBuilder{
		template: &template{
			Provider: provider,
		},
		configVariablesClient: configVariablesClient,
	}
}

func (b *templateBuilder) initFromRepository(version string, path string, rawyaml []byte, flavor string, bootstrap string) {

	// inspect the yaml for variables
	variables := inspectVariables(rawyaml)

	b.template.version = version
	b.template.flavor = flavor
	b.template.bootstrap = bootstrap
	b.template.variables = variables
	b.template.path = path
	b.template.rawyaml = rawyaml
}

func (b *templateBuilder) BuildFor(targetNamespace string, options TemplateOptions) error {

	// Replace variables
	yaml, err := replaceVariables(b.template.rawyaml, b.template.variables, b.configVariablesClient)
	if err != nil {
		return errors.Wrap(err, "failed to perform variable substitution")
	}

	// executes go templates
	yaml, err = execGoTemplates(yaml, options)
	if err != nil {
		return err
	}

	// set target targetNamespace for all objects using kustomize
	yamlFile := "template.yaml"

	// gets a kustomization file for setting targetNamespace
	kustomization := getKustomizationFile(targetNamespace, yamlFile, nil)

	// run kustomize build to get the final yaml
	finalYaml, err := kustomizeBuild(map[string][]byte{
		yamlFile:             yaml,
		"kustomization.yaml": []byte(kustomization),
	}, "")
	if err != nil {
		return errors.Wrap(err, "failed to kustomize template namespace")
	}

	yaml = finalYaml

	b.template.targetNamespace = targetNamespace
	b.template.yaml = yaml

	return nil
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

func getKustomizationFile(targetNamespace, fileName string, labels map[string]string) string {
	var sb strings.Builder
	sb.WriteString("apiVersion: kustomize.config.k8s.io/v1beta1\n")
	sb.WriteString("kind: Kustomization\n")

	sb.WriteString(fmt.Sprintf("namespace: \"%s\"\n", targetNamespace))

	if len(labels) > 0 {
		sb.WriteString("commonLabels:\n")
		for k, v := range labels {
			sb.WriteString(fmt.Sprintf("  %s: \"%s\"\n", k, v))
		}
	}

	sb.WriteString("resources:\n")
	sb.WriteString(fmt.Sprintf("- \"%s\"\n", fileName))

	return sb.String()
}
