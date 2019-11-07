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

package test

import (
	"github.com/pkg/errors"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/yaml"
)

type FakeReader struct {
	initialized bool
	variables   map[string]string
	providers   []configProvider
}

type configProvider struct {
	Name string                    `json:"name,omitempty"`
	URL  string                    `json:"url,omitempty"`
	Type clusterctlv1.ProviderType `json:"type,omitempty"`
}

func (f *FakeReader) Init(config string) error {
	f.initialized = true
	return nil
}

func (f *FakeReader) GetString(key string) (string, error) {
	if val, ok := f.variables[key]; ok {
		return val, nil
	}
	return "", errors.Errorf("value for variable %q is not set", key)
}

func (f *FakeReader) UnmarshalKey(key string, rawval interface{}) error {
	data, err := f.GetString(key)
	if err != nil {
		return nil
	}
	return yaml.Unmarshal([]byte(data), rawval)
}

func NewFakeReader() *FakeReader {
	return &FakeReader{
		variables: map[string]string{},
	}
}

func (f *FakeReader) WithVar(key, value string) *FakeReader {
	f.variables[key] = value
	return f
}

func (f *FakeReader) WithProvider(name string, ttype clusterctlv1.ProviderType, url string) *FakeReader {
	f.providers = append(f.providers, configProvider{
		Name: name,
		URL:  url,
		Type: ttype,
	})

	yaml, _ := yaml.Marshal(f.providers)
	f.variables["providers"] = string(yaml)

	return f
}
