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
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/repository"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ComponentsClient has methods to work with provider components in the cluster.
type ComponentsClient interface {
	Create(components repository.Components) error
	ScaleDownControllers(provider clusterctlv1.Provider) error
	Delete(provider clusterctlv1.Provider, forceDeleteNamespace, forceDeleteCRD bool) error
	Pivot(to Client) error
}

// providerComponents implements ComponentsClient.
type providerComponents struct {
	k8sproxy K8SProxy
}

// ensure providerComponents implements ComponentsClient.
var _ ComponentsClient = &providerComponents{}

// Create provider components defined in the yaml file.
func (p *providerComponents) Create(components repository.Components) error {

	c, err := p.k8sproxy.NewClient()
	if err != nil {
		return err
	}

	// sort provider components for creation according to relation across objects (e.g. Namespace before everything namespaced)
	resources := sortResourcesForCreate(components.Objs())

	// creates (or updates) provider components
	for _, r := range resources {

		// check if the component already exists, and eventually update it
		currentR := &unstructured.Unstructured{}
		currentR.SetGroupVersionKind(r.GroupVersionKind())

		key := client.ObjectKey{
			Namespace: r.GetNamespace(),
			Name:      r.GetName(),
		}
		if err = c.Get(ctx, key, currentR); err != nil {
			if !apierrors.IsNotFound(err) {
				return errors.Wrapf(err, "failed to get current provider object")
			}

			//if it does not exists, create the component
			klog.V(3).Infof("Creating: %s, %s/%s", r.GroupVersionKind(), r.GetNamespace(), r.GetName())
			if err = c.Create(ctx, &r); err != nil { //nolint
				return errors.Wrapf(err, "failed to create provider object")
			}

			continue
		}

		// otherwise update the component
		klog.V(3).Infof("Updating: %s, %s/%s", r.GroupVersionKind(), r.GetNamespace(), r.GetName())

		// if upgrading an existing component, then use the current resourceVersion for the optimistic lock
		r.SetResourceVersion(currentR.GetResourceVersion())
		if err = c.Update(ctx, &r); err != nil { //nolint
			return errors.Wrapf(err, "failed to update provider object")
		}
	}

	return nil
}

func (p *providerComponents) ScaleDownControllers(provider clusterctlv1.Provider) error {
	cs, err := p.k8sproxy.NewClient()
	if err != nil {
		return err
	}

	// Get all the Deployments linked to the provider
	selectors := []client.ListOption{
		client.MatchingLabels{
			clusterctlv1.ClusterctlProviderLabelName: provider.Name,
		},
		client.InNamespace(provider.Namespace),
	}

	deploymentList := &appsv1.DeploymentList{}
	if err := cs.List(ctx, deploymentList, selectors...); err != nil {
		return errors.Wrapf(err, "error listing Deployments for Provider %s/%s", provider.Namespace, provider.Name)
	}

	// For each deployment, if the number of replicas is different than the expected, then scale the deployment
	for _, deployment := range deploymentList.Items {
		if deployment.Spec.Replicas != nil && *deployment.Spec.Replicas == 0 {
			klog.V(3).Infof("Scaling down: %s/%s already has 0 replicas", deployment.Namespace, deployment.Name)
			continue
		}

		klog.V(3).Infof("Scaling down: %s/%s (from %d to %d replicas)", deployment.Namespace, deployment.Name, *deployment.Spec.Replicas, 0)
		if err := p.k8sproxy.ScaleDeployment(&deployment, 0); err != nil { //nolint
			return errors.Wrapf(err, "Failed to scale down %s/%s", deployment.Namespace, deployment.Name)
		}
	}

	return nil
}

func (p *providerComponents) Delete(provider clusterctlv1.Provider, forceDeleteNamespace, forceDeleteCRD bool) error {
	klog.V(2).Infof("Deleting Provider %s/%s, delete ns=%t, delete crd=%t", provider.Namespace, provider.Name, forceDeleteNamespace, forceDeleteCRD)

	// if we are not deleting the CRD (and the related objects controlled by the provider)
	// the scale down the controller so we give chance to exit from the reconcile loop in a controlled way
	if !forceDeleteCRD {
		err := p.ScaleDownControllers(provider)
		if err != nil {
			return err
		}
	}

	// fetch all the resources belonging to a provider
	//
	labels := map[string]string{
		clusterctlv1.ClusterctlProviderLabelName: provider.Name,
	}
	resources, err := p.k8sproxy.ListResources(provider.Namespace, labels)
	if err != nil {
		return err
	}

	// if we are preserving the CRD (and the related objects controlled by the provider),
	// remove CRD from the list of objects to delete
	if !forceDeleteCRD {
		var resourcesToDelete []unstructured.Unstructured
		for _, r := range resources {
			if r.GroupVersionKind().Kind != "CustomResourceDefinition" {
				resourcesToDelete = append(resourcesToDelete, r)
			}
		}
		resources = resourcesToDelete
	}

	// if we are preserving the Namespace were the provider is hosted (and the related objects),
	// remove the Namespace from the list of objects to delete
	if !forceDeleteNamespace {
		var resourcesToDelete []unstructured.Unstructured
		for _, r := range resources {
			if r.GroupVersionKind().Kind != "Namespace" {
				resourcesToDelete = append(resourcesToDelete, r)
			}
		}
		resources = resourcesToDelete
	} else {
		// if we are deleting the Namespace were the provider is hosted,
		// we can remove Namespaced objects from the list of objects to delete because
		// everything that is contained in the namespace will be deleted by the Namespace controller
		var namespaces []string
		for _, r := range resources {
			if r.GroupVersionKind().Kind == "Namespace" {
				namespaces = append(namespaces, r.GetName())
			}
		}

		var resourcesToDelete []unstructured.Unstructured
		for _, r := range resources {
			isInNamespace := false
			for _, n := range namespaces {
				if n == r.GetNamespace() {
					isInNamespace = true
					break
				}
			}
			if !isInNamespace {
				resourcesToDelete = append(resourcesToDelete, r)
			}
		}
		resources = resourcesToDelete
	}

	// delete all the provider components
	cs, err := p.k8sproxy.NewClient()
	if err != nil {
		return err
	}

	for _, obj := range resources {
		klog.V(3).Infof("Deleting: %s, %s/%s", obj.GroupVersionKind(), obj.GetNamespace(), obj.GetName())
		err := cs.Delete(ctx, &obj) //nolint
		if err != nil {
			// tolerate IsNotFound error that might happen because we are not enforcing a deletion order
			// that considers relation across objects (e.g. Deployments -> ReplicaSets -> Pods)
			if apierrors.IsNotFound(err) {
				continue
			}
			return err
		}
	}

	return nil
}

func (p *providerComponents) Pivot(to Client) error {
	from := p

	// fetch all the resources controlled by clusterctl
	labels := map[string]string{
		clusterctlv1.ClusterctlLabelName: "",
	}

	resources, err := from.k8sproxy.ListResources("", labels)
	if err != nil {
		return err
	}

	// sort provider components for creation according to relation across objects (e.g. Namespace before everything namespaced)
	resources = sortResourcesForCreate(resources)

	csTo, err := to.K8SProxy().NewClient()
	if err != nil {
		return err
	}

	// creates (or updates) provider components in the target cluster
	for _, r := range resources {

		// fix resource for pivot (e.g cleanup ClusterIP for services)
		if err := fixResourceForPivot(&r); err != nil { //nolint
			return err
		}

		// check if the component already exists, and eventually update it
		currentR := &unstructured.Unstructured{}
		currentR.SetGroupVersionKind(r.GroupVersionKind())

		key := client.ObjectKey{
			Namespace: r.GetNamespace(),
			Name:      r.GetName(),
		}
		if err = csTo.Get(ctx, key, currentR); err != nil {
			if !apierrors.IsNotFound(err) {
				return errors.Wrapf(err, "failed to get current provider object")
			}

			klog.V(3).Infof("Updating: %s, %s/%s", r.GroupVersionKind(), r.GetNamespace(), r.GetName())

			// if upgrading an existing component, pick up the UID from the target cluster
			r.SetUID(currentR.GetUID())
			// if upgrading an existing component, then use the current resourceVersion for the optimistic lock
			r.SetResourceVersion(currentR.GetResourceVersion())

			if err = csTo.Update(ctx, &r); err != nil { //nolint
				return errors.Wrapf(err, "failed to update provider object")
			}
		}

		// otherwise create the component
		klog.V(3).Infof("Creating: %s, %s/%s", r.GroupVersionKind(), r.GetNamespace(), r.GetName())
		if err = csTo.Create(ctx, &r); err != nil { //nolint
			return errors.Wrapf(err, "failed to create provider object")
		}
	}

	return nil
}

func fixResourceForPivot(r *unstructured.Unstructured) error {
	// cleanup current resource version because it does not makes sense in the target cluster
	r.SetResourceVersion("")

	// if the resource is a Service of type ClusterIP, cleanup the cluster IP so a new one will be assigned in the target cluster
	if r.GetKind() == "Service" {
		//TODO: refactor and cast to service instead of working on unstructured

		c := r.UnstructuredContent()

		val, found, err := unstructured.NestedString(c, "spec", "type")
		if err != nil {
			return err
		}

		if !found {
			klog.V(3).Info("spec.type not found...")
			return nil
		}

		if val == "ClusterIP" {
			klog.V(3).Info("cleaning spec.clusterIP ...")
			err := unstructured.SetNestedField(c, "", "spec", "clusterIP")
			if err != nil {
				return err
			}
		}

		r.SetUnstructuredContent(c)
	}

	return nil
}

// newComponentsClient returns a providerComponents.
func newComponentsClient(k8sproxy K8SProxy) *providerComponents {
	return &providerComponents{
		k8sproxy: k8sproxy,
	}
}

//TODO: instead of sorting, cluster resources in prioritized buckets and run in parallel

// - Namespaces go first because all namespaced resources depend on them.
// - Custom Resource Definitions come before Custom Resource so that they can be
//   restored with their corresponding CRD.
// - Storage Classes are needed to create PVs and PVCs correctly.
// - PVs go before PVCs because PVCs depend on them.
// - PVCs go before pods or controllers so they can be mounted as volumes.
// - Secrets and config maps go before pods or controllers so they can be mounted as volumes.
// - Service accounts go before pods or controllers so pods can use them.
// - Limit ranges go before pods or controllers so pods can use them.
// - Pods go before ReplicaSets
// - ReplicaSets go before Deployments
// - Endpoints go before Services
var defaultCreatePriorities = []string{
	"Namespace",
	"CustomResourceDefinition",
	"StorageClass",
	"PersistentVolume",
	"PersistentVolumeClaim",
	"Secret",
	"ConfigMap",
	"ServiceAccount",
	"LimitRange",
	"Pods",
	"ReplicaSet",
	"Endpoints",
}

func sortResourcesForCreate(resources []unstructured.Unstructured) []unstructured.Unstructured {
	var ret []unstructured.Unstructured

	// First get resources by priority
	for _, p := range defaultCreatePriorities {
		for _, o := range resources {
			if o.GetKind() == p {
				ret = append(ret, o)
			}
		}
	}

	// Then get all the other resources
	for _, o := range resources {
		found := false
		for _, r := range ret {
			if o.GroupVersionKind() == r.GroupVersionKind() && o.GetNamespace() == r.GetNamespace() && o.GetName() == r.GetName() {
				found = true
				break
			}
		}
		if !found {
			ret = append(ret, o)
		}
	}

	return ret
}
