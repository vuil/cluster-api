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

// NewOptions carries the options supported by New
type NewOptions struct {
	injectReader Reader
}

// Option is a configuration option supplied to New
type Option func(*NewOptions)

// ControlPlanes sets the number of control plane nodes for create
func InjectReader(reader Reader) Option {
	return func(c *NewOptions) {
		c.injectReader = reader
	}
}

// New returns a Client.
func New(path string, options ...Option) (Client, error) {
	return newConfigClient(path, options...)
}

func newConfigClient(path string, options ...Option) (*configClient, error) {
	cfg := &NewOptions{}
	for _, o := range options {
		o(cfg)
	}

	reader := cfg.injectReader
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

// Reader has methods to read configurations.
type Reader interface {
	Init(string) error
	GetString(string) (string, error)
	UnmarshalKey(string, interface{}) error
}

// Ensures the FakeReader implements reader
var _ Reader = &test.FakeReader{}
