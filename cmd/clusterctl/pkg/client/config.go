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

package client

import (
	"unsafe"

	"github.com/pkg/errors"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/repository"
)

func (c *clusterctlClient) GetProvidersConfig() ([]Provider, error) {
	r, err := c.configClient.Providers().List()
	if err != nil {
		return nil, err
	}

	// Provider is an alias for config.Provider; this makes low level conversion
	rr := *(*[]Provider)(unsafe.Pointer(&r))

	return rr, nil
}

func (c *clusterctlClient) GetProviderConfig(provider, targetNameSpace, watchingNamespace string) (Components, error) {
	components, err := c.getComponentsByName(provider, targetNameSpace, watchingNamespace)
	if err != nil {
		return nil, err
	}

	return components, nil
}

func (c *clusterctlClient) GetClusterTemplate(kubeconfig, provider, flavor, bootstrap string, options TemplateOptions) (Template, error) {

	cluster, err := c.clusterClientFactory(kubeconfig)
	if err != nil {
		return nil, err
	}

	if _, err := cluster.ProviderMetadata().EnsureMetadata(); err != nil {
		return nil, err
	}

	if provider == "" {
		provider, err = cluster.ProviderMetadata().GetDefaultProvider(clusterctlv1.InfrastructureProviderType)
		if err != nil {
			return nil, err
		}

		if provider == "" {
			return nil, errors.New("failed to identify the default infrastructure provider. Please specify an infrastructure provider")
		}
	}

	namespace, name, version, err := parseProviderName(provider)
	if err != nil {
		return nil, err
	}

	if version == "" {
		if namespace != "" {
			ps, err := cluster.ProviderMetadata().Get(namespace, name)
			if err != nil {
				return nil, err
			}
			version = ps.Version
		} else {
			v, err := cluster.ProviderMetadata().GetDefaultVersion(name)
			if err != nil {
				return nil, err
			}

			if v == "" {
				return nil, errors.Errorf("failed to identify the version for the provider %q. Please specify a version", name)
			}
			version = v
		}
	}

	if options.Namespace == "" {
		ns, err := cluster.K8SProxy().CurrentNamespace()
		if err != nil {
			return nil, err
		}
		options.Namespace = ns
	}

	providerConfig, err := c.configClient.Providers().Get(name)
	if err != nil {
		return nil, err
	}

	repo, err := c.repositoryClientFactory(*providerConfig)
	if err != nil {
		return nil, err
	}

	roptions := *(*repository.TemplateOptions)(unsafe.Pointer(&options))

	template, err := repo.Templates(version).Get(flavor, bootstrap, roptions)
	if err != nil {
		return nil, err
	}
	return template, nil
}
