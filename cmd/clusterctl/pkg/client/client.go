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
	"strings"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/cluster"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/repository"
)

type Provider config.Provider

type Components repository.Components

type Template repository.Template

type TemplateOptions repository.TemplateOptions

const CAPI = config.CAPI

// Client is used to interact with the clusterctl client library
type Client interface {
	GetProvidersConfig() ([]Provider, error)
	GetProviderConfig(provider, targetNameSpace, watchingNamespace string) (Components, error)
	Init(kubeconfig string, coreProvider string, bootstrapProviders, infrastructureProviders []string, targetNameSpace, watchingNamespace string, force bool) ([]Components, bool, error)
	GetClusterTemplate(kubeconfig, provider, flavor, bootstrap string, options TemplateOptions) (Template, error)
	Pivot(fromKubeconfig, toKubeconfig string) error
	Delete(kubeconfig string, forceDeleteNamespace, forceDeleteCRD bool, args ...string) error
}

// clusterctlClient implements Client.
type clusterctlClient struct {
	configClient            config.Client
	repositoryClientFactory RepositoryClientFactory
	clusterClientFactory    ClusterClientFactory
}

// ensure clusterctlClient implements Client.
var _ Client = &clusterctlClient{}

func (c *clusterctlClient) getComponentsByName(provider string, targetNameSpace string, watchingNamespace string) (repository.Components, error) {
	namespace, name, version, err := parseProviderName(provider)
	if err != nil {
		return nil, err
	}

	if targetNameSpace != "" {
		namespace = targetNameSpace
	}

	providerConfig, err := c.configClient.Providers().Get(name)
	if err != nil {
		return nil, err
	}

	repository, err := c.repositoryClientFactory(*providerConfig)
	if err != nil {
		return nil, err
	}

	components, err := repository.Components().Get(version, namespace, watchingNamespace)
	if err != nil {
		return nil, err
	}
	return components, nil
}

// New returns a configClient.
func New(path string, options Options) (Client, error) {
	return newClusterctlClient(path, options)
}

func newClusterctlClient(path string, options Options) (*clusterctlClient, error) {
	configClient := options.InjectConfig
	if configClient == nil {
		c, err := config.New(path, config.Options{})
		if err != nil {
			return nil, err
		}
		configClient = c
	}
	if options.InjectRepositoryFactory == nil {
		options.InjectRepositoryFactory = func(providerConfig config.Provider) (repository.Client, error) {
			return repository.New(providerConfig, configClient.Variables(), repository.Options{})
		}
	}
	if options.InjectClusterFactory == nil {
		options.InjectClusterFactory = func(kubeconfig string) (cluster.Client, error) {
			return cluster.New(kubeconfig, cluster.Options{}), nil
		}
	}
	return &clusterctlClient{
		configClient:            configClient,
		repositoryClientFactory: options.InjectRepositoryFactory,
		clusterClientFactory:    options.InjectClusterFactory,
	}, nil
}

// Options allow to set clusterctlClient options
type Options struct {
	InjectConfig            config.Client
	InjectRepositoryFactory RepositoryClientFactory
	InjectClusterFactory    ClusterClientFactory
}

type RepositoryClientFactory func(config.Provider) (repository.Client, error)
type ClusterClientFactory func(string) (cluster.Client, error)

func parseProviderName(provider string) (namespace string, name string, version string, err error) {
	t1 := strings.Split(strings.ToLower(provider), "/")
	if len(t1) > 2 {
		return "", "", "", errors.Errorf("invalid provider name %q. Provider name should be in the form [namespace/]name[:version]", provider)
	}

	if len(t1) == 2 {
		namespace = t1[0]
		provider = t1[1]
	}

	t2 := strings.Split(strings.ToLower(provider), ":")
	if len(t2) > 2 {
		return "", "", "", errors.Errorf("invalid provider name %q. Provider name should be in the form [namespace/]name[:version]", provider)
	}

	name = t2[0]
	errs := validation.IsDNS1123Label(name)
	if len(errs) != 0 {
		return "", "", "", errors.Errorf("invalid name value: %s", strings.Join(errs, "; "))
	}

	version = ""
	if len(t2) > 1 {
		version = t2[1]
	}

	return namespace, name, version, nil
}
