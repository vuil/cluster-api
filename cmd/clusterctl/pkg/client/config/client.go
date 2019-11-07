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

package config

import (
	"github.com/pkg/errors"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/test"
)

// Client is used to interact with the clusterctl configuration.
type Client interface {
	Providers() ProvidersClient
	Variables() VariablesClient
}

// configClient implements Client.
type configClient struct {
	reader Reader
}

// ensure configClient implements Client.
var _ Client = &configClient{}

// Providers provide access to provider configurations.
func (c *configClient) Providers() ProvidersClient {
	return newProvidersClient(c.reader)
}

// Variables provide access to variablesClient configurations.
func (c *configClient) Variables() VariablesClient {
	return newVariablesClient(c.reader)
}

// New returns a Client.
func New(path string, options Options) (Client, error) {
	return newConfigClient(path, options)
}

func newConfigClient(path string, options Options) (*configClient, error) {
	reader := options.InjectReader
	if reader == nil {
		reader = newViperReader()
	}

	if err := reader.Init(path); err != nil {
		return nil, errors.Wrap(err, "failed to initialize the configuration reader")
	}

	return &configClient{
		reader: reader,
	}, nil
}

// Options allow to set options for a Client
type Options struct {
	InjectReader Reader
}

// Reader has methods to read configurations.
type Reader interface {
	Init(string) error
	GetString(string) (string, error)
	UnmarshalKey(string, interface{}) error
}

var _ Reader = &test.FakeReader{}
