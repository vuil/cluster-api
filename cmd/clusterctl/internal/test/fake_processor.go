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

import "sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"

type FakeProcessor struct {
	errGetVariables error
	artifactName    string
}

func NewFakeProcessor() *FakeProcessor {
	return &FakeProcessor{}
}

func (fp *FakeProcessor) WithArtifactName(n string) *FakeProcessor {
	fp.artifactName = n
	return fp
}

func (fp *FakeProcessor) WithGetVariablesErr(e error) *FakeProcessor {
	fp.errGetVariables = e
	return fp
}

func (fp *FakeProcessor) ArtifactName(version, flavor string) string {
	return fp.artifactName
}

func (fp *FakeProcessor) GetVariables(raw []byte) ([]string, error) {
	return nil, fp.errGetVariables
}

func (fp *FakeProcessor) Process(raw []byte, variablesClient config.VariablesClient) ([]byte, error) {
	return nil, nil
}
