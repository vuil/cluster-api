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
	"time"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type FakeK8SProxy struct {
	cs   client.Client
	objs []runtime.Object
}

func (f *FakeK8SProxy) CurrentNamespace() (string, error) {
	return "default", nil
}

func (f *FakeK8SProxy) NewClient() (client.Client, error) {
	if f.cs != nil {
		return f.cs, nil
	}
	f.cs = fake.NewFakeClientWithScheme(scheme.Scheme, f.objs...)

	return f.cs, nil
}

func (f *FakeK8SProxy) ListResources(namespace string, labels map[string]string) ([]unstructured.Unstructured, error) {
	// returning all the resources known by the FakeK8SProxy
	var ret []unstructured.Unstructured //nolint
	for _, o := range f.objs {
		u := unstructured.Unstructured{}
		scheme.Scheme.Convert(o, &u, nil)

		// filter by namespace, if any
		if namespace != "" && u.GetNamespace() != "" && u.GetNamespace() != namespace {
			continue
		}

		// filter by label, if any
		haslabel := false
		for l, v := range labels {
			for ul, uv := range u.GetLabels() {
				if l == ul && v == uv {
					haslabel = true
				}
			}
		}
		if !haslabel {
			continue
		}

		ret = append(ret, u)
	}

	return ret, nil
}

func (f *FakeK8SProxy) ScaleDeployment(deployment *appsv1.Deployment, replicas int32) error {
	// TODO: move ScaleDeployment code from k8sproxy to util and make it testable
	return nil
}

func (f *FakeK8SProxy) PollImmediate(interval, timeout time.Duration, condition func() (done bool, err error)) error {
	// Unit test should not wait; additionally, there are no controllers running, so condition will never pass during test
	return nil
}

func NewFakeK8SProxy() *FakeK8SProxy {
	return &FakeK8SProxy{}
}

func (f *FakeK8SProxy) WithObjs(objs ...runtime.Object) *FakeK8SProxy {
	f.objs = objs
	return f
}
