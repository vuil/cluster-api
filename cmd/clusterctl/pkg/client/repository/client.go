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
	"net/url"

	"github.com/pkg/errors"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/test"
)

// Client is used to interact with provider repositories.
type Client interface {
	config.Provider
	DefaultVersion() string
	Components() ComponentsClient
	Templates(version string) TemplatesClient
}

// repositoryClient implements Client.
type repositoryClient struct {
	config.Provider
	configVariablesClient config.VariablesClient
	repository            Repository
}

// ensure repositoryClient implements Client.
var _ Client = &repositoryClient{}

func (c *repositoryClient) DefaultVersion() string {
	return c.repository.DefaultVersion()
}

// Components provide access to yaml files for generating provider componentsClient.
func (c *repositoryClient) Components() ComponentsClient {
	return newComponentsClient(c.Provider, c.repository, c.configVariablesClient)
}

// Templates provide access to yaml files for cluster templatesClient.
func (c *repositoryClient) Templates(version string) TemplatesClient {
	return newTemplatesClient(c.Provider, version, c.repository, c.configVariablesClient)
}

// New returns a Client.
func New(provider config.Provider, configVariablesClient config.VariablesClient, options Options) (Client, error) {
	return newRepositoryClient(provider, configVariablesClient, options)
}

func newRepositoryClient(provider config.Provider, configVariablesClient config.VariablesClient, options Options) (*repositoryClient, error) {
	repository := options.InjectRepository
	if repository == nil {
		r, err := repositoryFactory(provider, configVariablesClient)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get repository client for %q", provider.Name())
		}
		repository = r
	}

	return &repositoryClient{
		Provider:              provider,
		repository:            repository,
		configVariablesClient: configVariablesClient,
	}, nil
}

// Options allow to set Client options
type Options struct {
	InjectRepository Repository
}

type Repository interface {
	DefaultVersion() string

	// All the paths returned by this interface should be relative to this path
	RootPath() string

	// A folder or file name
	ComponentsPath() string

	// In case ComponentsPath has nested folders
	KustomizeDir() string

	GetFiles(version string, path string) (map[string][]byte, error)
}

var _ Repository = &test.FakeRepository{}

func repositoryFactory(providerConfig config.Provider, configVariablesClient config.VariablesClient) (Repository, error) {
	// parse the repository url
	rURL, err := url.Parse(providerConfig.URL())
	if err != nil {
		return nil, errors.Errorf("failed to parse repository url %q", providerConfig.URL())
	}

	// if the url is a github repository
	if rURL.Scheme == httpsScheme && rURL.Host == githubDomain {
		repo, err := newGitHubRepositoryImpl(providerConfig, configVariablesClient)
		if err != nil {
			return nil, errors.Wrap(err, "error creating the GitHub repository client")
		}
		return repo, err
	}

	// if the url is a http/https repository
	if rURL.Scheme == httpScheme || rURL.Scheme == httpsScheme {
		//TODO: implement http/https provider!
		panic("TODO: implement http/https provider!")
	}

	// if the url is a local repository
	if rURL.Scheme == "" || rURL.Scheme == fileScheme {
		repo, err := newFilesystemRepositoryImpl(providerConfig)
		if err != nil {
			return nil, errors.Wrap(err, "error creating the filesystem repository client")
		}
		return repo, err
	}

	return nil, errors.Errorf("invalid provider url. there are no provider implementation for %q schema", rURL.Scheme)
}
