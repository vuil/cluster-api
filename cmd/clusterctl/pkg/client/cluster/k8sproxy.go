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

package cluster

import (
	"time"

	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	Scheme = scheme.Scheme
)

type k8sproxy struct {
	kubeconfig string
}

var _ K8SProxy = &k8sproxy{}

// CurrentNamespace returns the namespace from the current context in the kubeconfig file
func (k *k8sproxy) CurrentNamespace() (string, error) {
	config, err := clientcmd.LoadFromFile(k.kubeconfig)
	if err != nil {
		return "", errors.Wrapf(err, "failed to load Kubeconfig file from %q", k.kubeconfig)
	}

	if config.CurrentContext == "" {
		return "", errors.Wrapf(err, "failed to get current-context from %q", k.kubeconfig)
	}

	v, ok := config.Contexts[config.CurrentContext]
	if !ok {
		return "", errors.Wrapf(err, "failed to get context %q from %q", config.CurrentContext, k.kubeconfig)
	}

	if v.Namespace != "" {
		return v.Namespace, nil
	}

	return "default", nil
}

func (k *k8sproxy) NewClient() (client.Client, error) {
	config, err := k.getConfig()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create controller-runtime client")
	}

	c, err := client.New(config, client.Options{Scheme: Scheme})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create controller-runtime client")
	}

	return c, nil
}

func (k *k8sproxy) newClientSet() (*kubernetes.Clientset, error) {
	config, err := k.getConfig()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create client-go client")
	}

	cs, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create client-go client")
	}

	return cs, nil
}

func (k *k8sproxy) ListResources(namespace string, labels map[string]string) ([]unstructured.Unstructured, error) {
	cs, err := k.newClientSet()
	if err != nil {
		return nil, err
	}

	c, err := k.NewClient()
	if err != nil {
		return nil, err
	}

	// get all the API resources in the cluster
	resourceList, err := cs.Discovery().ServerPreferredResources()
	if err != nil {
		return nil, errors.Wrap(err, "failed to list api resources")
	}

	var ret []unstructured.Unstructured
	for _, resourceGroup := range resourceList {
		for _, resourceKind := range resourceGroup.APIResources {
			// Check if the resource has list and delete methods
			// (list has is required by this method, delete by the callers of this method)
			hasList := false
			hasDelete := false
			for _, v := range resourceKind.Verbs {
				if v == "delete" {
					hasDelete = true
				}
				if v == "list" {
					hasList = true
				}
			}
			if !(hasList && hasDelete) {
				continue
			}

			// Filters out Kinds that exists in two api groups (we are excluding one of the two groups arbitrarily)
			if resourceGroup.GroupVersion == "extensions/v1beta1" &&
				(resourceKind.Name == "daemonsets" || resourceKind.Name == "deployments" || resourceKind.Name == "replicasets" || resourceKind.Name == "networkpolicies" || resourceKind.Name == "ingresses") {
				continue
			}

			// List all the instances of this Kind
			selectors := []client.ListOption{
				client.MatchingLabels(labels),
			}

			if namespace != "" && resourceKind.Namespaced {
				selectors = append(selectors, client.InNamespace(namespace))
			}

			objList := new(unstructured.UnstructuredList)
			objList.SetAPIVersion(resourceGroup.GroupVersion)
			objList.SetKind(resourceKind.Kind)

			if err := c.List(ctx, objList, selectors...); err != nil {
				if apierrors.IsNotFound(err) {
					continue
				}
				return nil, errors.Wrapf(err, "failed to list %q resources", objList.GroupVersionKind())
			}

			// Add obj to the result
			for _, obj := range objList.Items {
				o := obj //pin
				ret = append(ret, o)
			}
		}
	}
	return ret, nil
}

func (k *k8sproxy) ScaleDeployment(deployment *appsv1.Deployment, replicas int32) error {
	cs, err := k.newClientSet()
	if err != nil {
		return err
	}

	deploymentClient := cs.AppsV1().Deployments(deployment.Namespace)

	if err := k.PollImmediate(retryAcquireClient, timeoutAcquireClient, func() (bool, error) {
		_, err := deploymentClient.UpdateScale(deployment.Name, &autoscalingv1.Scale{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: deployment.Namespace,
				Name:      deployment.Name,
			},
			Spec: autoscalingv1.ScaleSpec{
				Replicas: replicas,
			},
		})
		if err != nil && !apierrors.IsNotFound(err) {
			return false, err
		}

		d, err := deploymentClient.Get(deployment.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		return d.Status.Replicas == replicas && d.Status.AvailableReplicas == replicas && d.Status.ReadyReplicas == replicas, nil
	}); err != nil {
		return errors.Wrapf(err, "failed to scale deployment %s/%s", deployment.Namespace, deployment.Name)
	}

	return nil
}

func (k *k8sproxy) PollImmediate(interval, timeout time.Duration, condition func() (done bool, err error)) error {
	return wait.PollImmediate(interval, timeout, condition)
}

func newK8SProxy(kubeconfig string) K8SProxy {
	// If a kubeconfig file is provided respect that, otherwise find a config file in the standard locations
	if kubeconfig == "" {
		kubeconfig = clientcmd.NewDefaultClientConfigLoadingRules().GetDefaultFilename()
	}

	return &k8sproxy{
		kubeconfig: kubeconfig,
	}
}

func (k *k8sproxy) getConfig() (*rest.Config, error) {
	config, err := clientcmd.LoadFromFile(k.kubeconfig)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load Kubeconfig file from %q", k.kubeconfig)
	}

	// Create a client config using the config.CurrentContext
	restConfig, err := clientcmd.NewDefaultClientConfig(*config, &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		return nil, errors.Wrap(err, "failed to rest client")
	}

	return restConfig, nil
}
