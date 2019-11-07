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

import "sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/test"

// VariablesClient has methods to work with variablesClient configuration.
type VariablesClient interface {
	Get(key string) (string, error)
}

var _ VariablesClient = &test.FakeVariableClient{}

// variablesClient implements VariablesClient.
type variablesClient struct {
	reader Reader
}

// ensure variablesClient implements VariablesClient.
var _ VariablesClient = &variablesClient{}

// newVariablesClient returns a variablesClient.
func newVariablesClient(reader Reader) *variablesClient {
	return &variablesClient{
		reader: reader,
	}
}

// Get return the value of the environment variable identified by key.
// In case the variablesClient does not exists, an error is returned.
func (p *variablesClient) Get(key string) (string, error) {
	return p.reader.GetString(key)
}
