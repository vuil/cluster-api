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
	"github.com/pkg/errors"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/config"
)

// ComponentsClient has methods to work with yaml file for generating providing componentsClient.
// Assets are yaml files to be used for deploying a provider into a management cluster.
type ComponentsClient interface {
	Get(version, targetNamespace, watchingNamespace string) (Components, error)
}

// componentsClient implements ComponentsClient.
type componentsClient struct {
	provider              config.Provider
	repository            Repository
	configVariablesClient config.VariablesClient
}

// ensure componentsClient implements ComponentsClient.
var _ ComponentsClient = &componentsClient{}

// newComponentsClient returns a componentsClient.
func newComponentsClient(provider config.Provider, repository Repository, configVariablesClient config.VariablesClient) *componentsClient {
	return &componentsClient{
		provider:              provider,
		repository:            repository,
		configVariablesClient: configVariablesClient,
	}
}

func (f *componentsClient) Get(version, targetNamespace, watchingNamespace string) (Components, error) {
	if version == "" {
		version = f.repository.DefaultVersion()
	}
	path := f.repository.ComponentsPath()

	files, err := f.repository.GetFiles(version, path)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read %q from the repository for provider %q", path, f.provider.Name())
	}

	if len(files) == 0 {
		return nil, errors.Errorf("the repository for provider %q does not contains %q file or folder", f.provider.Name(), path)
	}

	/*
		b := newComponentsBuilder(f.provider, f.configVariablesClient)

		err = b.initFromRepository(version, files, f.repository.KustomizeDir())
		if err != nil {
			return nil, errors.Wrapf(err, "failed parse components for provider %q", f.provider.Name())
		}

		err = b.buildFor(targetNamespace, watchingNamespace)
		if err != nil {
			return nil, errors.Wrapf(err, "failed kustomize targetNamespace and labels into %q for provider %q", path, f.provider.Name())
		}

		return b.components, nil
	*/

	return newComponents(f.provider, version, files, f.repository.KustomizeDir(), f.configVariablesClient, targetNamespace, watchingNamespace)
}
