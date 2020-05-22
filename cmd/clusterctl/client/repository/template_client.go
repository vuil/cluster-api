/*
Copyright 2020 The Kubernetes Authors.

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
	"github.com/pkg/errors"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/util"
	logf "sigs.k8s.io/cluster-api/cmd/clusterctl/log"
)

// TemplateClient has methods to work with cluster templates hosted on a provider repository.
// Templates are yaml files to be used for creating a guest cluster.
type TemplateClient interface {
	Get(flavor, targetNamespace string, listVariablesOnly bool) (Template, error)
}

// YamlProcessor defines the methods necessary for creating a specific yaml
// processor.
type YamlProcessor interface {

	// ArtifactName returns the name of the template artifacts that need to be
	// retrieved from the source.
	ArtifactName(version, flavor string) string

	// GetVariables parses the template artifact blob of bytes and provides a
	// list of variables that the template requires.
	GetVariables([]byte) ([]string, error)

	// Process processes the artifact blob of bytes and will return the final
	// yaml with values retrieved from the config.VariablesClient.
	Process([]byte, config.VariablesClient) ([]byte, error)
}

// templateClient implements TemplateClient.
type templateClient struct {
	provider              config.Provider
	version               string
	repository            Repository
	configVariablesClient config.VariablesClient
	processor             YamlProcessor
}

type TemplateClientInput struct {
	provider              config.Provider
	repository            Repository
	configVariablesClient config.VariablesClient
}

// Ensure templateClient implements the TemplateClient interface.
var _ TemplateClient = &templateClient{}

// TemplateClientOption is a configuration option supplied to
// newTemplateClient
type TemplateClientOption func(*templateClient)

// InjectYamlProcessor allows to override the yaml processor implementation to use;
// by default, the SimpleYamlProcessor is used.
func InjectYamlProcessor(p YamlProcessor) TemplateClientOption {
	return func(c *templateClient) {
		c.processor = p
	}
}

// newTemplateClient returns a templateClient. It uses the SimpleYamlProcessor
// by default
func newTemplateClient(input TemplateClientInput, version string, opts ...TemplateClientOption) *templateClient {
	tc := &templateClient{
		provider:              input.provider,
		version:               version,
		repository:            input.repository,
		configVariablesClient: input.configVariablesClient,
		processor:             newSimpleYamlProcessor(),
	}

	for _, o := range opts {
		o(tc)
	}

	return tc
}

// Get return the template for the flavor specified.
// In case the template does not exists, an error is returned.
// Get assumes the following naming convention for templates: cluster-template[-<flavor_name>].yaml
func (c *templateClient) Get(flavor, targetNamespace string, listVariablesOnly bool) (Template, error) {
	log := logf.Log

	if targetNamespace == "" {
		return nil, errors.New("invalid arguments: please provide a targetNamespace")
	}

	version := c.version
	name := c.processor.ArtifactName(version, flavor)

	// read the component YAML, reading the local override file if it exists, otherwise read from the provider repository
	rawArtifact, err := getLocalOverride(&newOverrideInput{
		configVariablesClient: c.configVariablesClient,
		provider:              c.provider,
		version:               version,
		filePath:              name,
	})
	if err != nil {
		return nil, err
	}

	if rawArtifact == nil {
		log.V(5).Info("Fetching", "File", name, "Provider", c.provider.ManifestLabel(), "Version", version)
		rawArtifact, err = c.repository.GetFile(version, name)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to read %q from provider's repository %q", name, c.provider.ManifestLabel())
		}
	} else {
		log.V(1).Info("Using", "Override", name, "Provider", c.provider.ManifestLabel(), "Version", version)
	}

	variables, err := c.processor.GetVariables(rawArtifact)
	if err != nil {
		return nil, err
	}

	if listVariablesOnly {
		return &template{
			variables:       variables,
			targetNamespace: targetNamespace,
		}, nil
	}

	processedYaml, err := c.processor.Process(rawArtifact, c.configVariablesClient)
	if err != nil {
		return nil, err
	}

	// Transform the yaml in a list of objects, so following transformation can work on typed objects (instead of working on a string/slice of bytes).
	objs, err := util.ToUnstructured(processedYaml)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse yaml")
	}

	// Ensures all the template components are deployed in the target namespace (applies only to namespaced objects)
	// This is required in order to ensure a cluster and all the related objects are in a single namespace, that is a requirement for
	// the clusterctl move operation (and also for many controller reconciliation loops).
	objs = fixTargetNamespace(objs, targetNamespace)

	return &template{
		objs:            objs,
		variables:       variables,
		targetNamespace: targetNamespace,
	}, nil
}
