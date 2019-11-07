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

	"github.com/pkg/errors"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/config"
)

// TemplatesClient has methods to work with templatesClient hosted on a provider repository.
// Templates are yaml files to be used for creating a guest cluster.
type TemplatesClient interface {
	Get(flavor, bootstrap string, options TemplateOptions) (Template, error)
}

// templatesClient implements TemplatesClient.
type templatesClient struct {
	provider              config.Provider
	version               string
	repository            Repository
	configVariablesClient config.VariablesClient
}

// ensure templatesClient implements TemplatesClient.
var _ TemplatesClient = &templatesClient{}

// newTemplatesClient returns a templatesClient.
func newTemplatesClient(provider config.Provider, version string, repository Repository, configVariablesClient config.VariablesClient) *templatesClient {
	return &templatesClient{
		provider:              provider,
		version:               version,
		repository:            repository,
		configVariablesClient: configVariablesClient,
	}
}

// Get return the template for the flavor/bootstrap provider specified.
// In case the template does not exists, an error is returned.
func (f *templatesClient) Get(flavor, bootstrap string, options TemplateOptions) (Template, error) {
	if bootstrap == "" {
		return nil, errors.New("invalid arguments: please proved a bootstrap provide name")
	}

	if options.Namespace == "" {
		return nil, errors.New("invalid arguments: please proved a targetNamespace")
	}

	// we are always reading templatesClient for a well know version, that usually is
	// the version of the provider installed in the management cluster.
	// ndr. "latest" does not apply for get.
	version := f.version

	// building template name according with following the convention:
	// config[-{flavor_name}]-{bootstrap_provider_name}.yaml
	name := "config"
	if flavor != "" {
		name = fmt.Sprintf("%s-%s", name, flavor)
	}
	name = fmt.Sprintf("%s-%s.yaml", name, bootstrap)

	files, err := f.repository.GetFiles(version, name)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read %q from the repository for provider %q", name, f.provider.Name())
	}

	if len(files) != 1 {
		return nil, errors.Errorf("the repository for provider %q does not contains %q file", f.provider.Name(), name)
	}
	content := files[name]

	t := newTemplateBuilder(f.provider, f.configVariablesClient)

	t.initFromRepository(version, name, content, flavor, bootstrap)

	err = t.BuildFor(options.Namespace, options)
	if err != nil {
		return nil, errors.Wrapf(err, "failed kustomize targetNamespace into template %q for provider %q", name, f.provider.Name())
	}

	return t.template, nil
}
