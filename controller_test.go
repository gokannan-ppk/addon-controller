/*
Copyright 2017 The Kubernetes Authors.

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

package main

import (
	"context"
	"reflect"
	"testing"
	"time"

	apps "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/diff"
	kubeinformers "k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2/ktesting"

	addoncontroller "github.com/gokannan-ppk/addon-controller/pkg/apis/addoncontroller/v1alpha1"
	"github.com/gokannan-ppk/addon-controller/pkg/generated/clientset/versioned/fake"
	informers "github.com/gokannan-ppk/addon-controller/pkg/generated/informers/externalversions"
)

var (
	alwaysReady        = func() bool { return true }
	noResyncPeriodFunc = func() time.Duration { return 0 }
)

type fixture struct {
	t *testing.T

	client     *fake.Clientset
	kubeclient *k8sfake.Clientset
	// Objects to put in the store.
	addonLister      []*addoncontroller.Addon
	deploymentLister []*apps.Deployment
	// Actions expected to happen on the client.
	kubeactions []core.Action
	actions     []core.Action
	// Objects from here preloaded into NewSimpleFake.
	kubeobjects []runtime.Object
	objects     []runtime.Object
}

func newFixture(t *testing.T) *fixture {
	f := &fixture{}
	f.t = t
	f.objects = []runtime.Object{}
	f.kubeobjects = []runtime.Object{}
	return f
}

func newAddon(name string, replicas *int32) *addoncontroller.Addon {
	return &addoncontroller.Addon{
		TypeMeta: metav1.TypeMeta{APIVersion: addoncontroller.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metav1.NamespaceDefault,
		},
		Spec: addoncontroller.AddonSpec{
			Versions: []addoncontroller.AddonVersion{
				addoncontroller.AddonVersion{
					Version:   "1.0.0",
					Changelog: "abc",
					Visible:   true,
				},
			},
		},
	}
}

func (f *fixture) newController(ctx context.Context) (*Controller, informers.SharedInformerFactory, kubeinformers.SharedInformerFactory) {
	f.client = fake.NewSimpleClientset(f.objects...)
	f.kubeclient = k8sfake.NewSimpleClientset(f.kubeobjects...)

	i := informers.NewSharedInformerFactory(f.client, noResyncPeriodFunc())
	k8sI := kubeinformers.NewSharedInformerFactory(f.kubeclient, noResyncPeriodFunc())

	c := NewController(ctx, f.kubeclient, f.client,
		k8sI.Apps().V1().Deployments(),
		k8sI.Core().V1().Services(),
		k8sI.Core().V1().ConfigMaps(),
		i.Addoncontroller().V1alpha1().Addons())
	c.addonsSynced = alwaysReady
	c.deploymentsSynced = alwaysReady
	c.recorder = &record.FakeRecorder{}

	for _, f := range f.addonLister {
		i.Addoncontroller().V1alpha1().Addons().Informer().GetIndexer().Add(f)
	}

	for _, d := range f.deploymentLister {
		k8sI.Apps().V1().Deployments().Informer().GetIndexer().Add(d)
	}

	return c, i, k8sI
}

func (f *fixture) run(ctx context.Context, addonName string) {
	f.runController(ctx, addonName, true, false)
}

func (f *fixture) runExpectError(ctx context.Context, addonName string) {
	f.runController(ctx, addonName, true, true)
}

func (f *fixture) runController(ctx context.Context, addonName string, startInformers bool, expectError bool) {
	c, i, k8sI := f.newController(ctx)
	if startInformers {
		i.Start(ctx.Done())
		k8sI.Start(ctx.Done())
	}

	err := c.syncHandler(ctx, addonName)
	if !expectError && err != nil {
		f.t.Errorf("error syncing addon: %v", err)
	} else if expectError && err == nil {
		f.t.Error("expected error syncing addon, got nil")
	}

	actions := filterInformerActions(f.client.Actions())
	for i, action := range actions {
		if len(f.actions) < i+1 {
			f.t.Errorf("%d unexpected actions: %+v", len(actions)-len(f.actions), actions[i:])
			break
		}

		expectedAction := f.actions[i]
		checkAction(expectedAction, action, f.t)
	}

	if len(f.actions) > len(actions) {
		f.t.Errorf("%d additional expected actions:%+v", len(f.actions)-len(actions), f.actions[len(actions):])
	}

	k8sActions := filterInformerActions(f.kubeclient.Actions())
	for i, action := range k8sActions {
		if len(f.kubeactions) < i+1 {
			f.t.Errorf("%d unexpected actions: %+v", len(k8sActions)-len(f.kubeactions), k8sActions[i:])
			break
		}

		expectedAction := f.kubeactions[i]
		checkAction(expectedAction, action, f.t)
	}

	if len(f.kubeactions) > len(k8sActions) {
		f.t.Errorf("%d additional expected actions:%+v", len(f.kubeactions)-len(k8sActions), f.kubeactions[len(k8sActions):])
	}
}

// checkAction verifies that expected and actual actions are equal and both have
// same attached resources
func checkAction(expected, actual core.Action, t *testing.T) {
	if !(expected.Matches(actual.GetVerb(), actual.GetResource().Resource) && actual.GetSubresource() == expected.GetSubresource()) {
		t.Errorf("Expected\n\t%#v\ngot\n\t%#v", expected, actual)
		return
	}

	if reflect.TypeOf(actual) != reflect.TypeOf(expected) {
		t.Errorf("Action has wrong type. Expected: %t. Got: %t", expected, actual)
		return
	}

	switch a := actual.(type) {
	case core.CreateActionImpl:
		e, _ := expected.(core.CreateActionImpl)
		expObject := e.GetObject()
		object := a.GetObject()

		if !reflect.DeepEqual(expObject, object) {
			t.Errorf("Action %s %s has wrong object\nDiff:\n %s",
				a.GetVerb(), a.GetResource().Resource, diff.ObjectGoPrintSideBySide(expObject, object))
		}
	case core.UpdateActionImpl:
		e, _ := expected.(core.UpdateActionImpl)
		expObject := e.GetObject()
		object := a.GetObject()

		if !reflect.DeepEqual(expObject, object) {
			t.Errorf("Action %s %s has wrong object\nDiff:\n %s",
				a.GetVerb(), a.GetResource().Resource, diff.ObjectGoPrintSideBySide(expObject, object))
		}
	case core.PatchActionImpl:
		e, _ := expected.(core.PatchActionImpl)
		expPatch := e.GetPatch()
		patch := a.GetPatch()

		if !reflect.DeepEqual(expPatch, patch) {
			t.Errorf("Action %s %s has wrong patch\nDiff:\n %s",
				a.GetVerb(), a.GetResource().Resource, diff.ObjectGoPrintSideBySide(expPatch, patch))
		}
	default:
		t.Errorf("Uncaptured Action %s %s, you should explicitly add a case to capture it",
			actual.GetVerb(), actual.GetResource().Resource)
	}
}

// filterInformerActions filters list and watch actions for testing resources.
// Since list and watch don't change resource state we can filter it to lower
// nose level in our tests.
func filterInformerActions(actions []core.Action) []core.Action {
	ret := []core.Action{}
	for _, action := range actions {
		if len(action.GetNamespace()) == 0 &&
			(action.Matches("list", "addons") ||
				action.Matches("watch", "addons") ||
				action.Matches("list", "deployments") ||
				action.Matches("watch", "deployments")) {
			continue
		}
		ret = append(ret, action)
	}

	return ret
}

func (f *fixture) expectCreateDeploymentAction(d *apps.Deployment) {
	f.kubeactions = append(f.kubeactions, core.NewCreateAction(schema.GroupVersionResource{Resource: "deployments"}, d.Namespace, d))
}

func (f *fixture) expectUpdateDeploymentAction(d *apps.Deployment) {
	f.kubeactions = append(f.kubeactions, core.NewUpdateAction(schema.GroupVersionResource{Resource: "deployments"}, d.Namespace, d))
}

func (f *fixture) expectUpdateAddonStatusAction(addon *addoncontroller.Addon) {
	action := core.NewUpdateSubresourceAction(schema.GroupVersionResource{Resource: "addons"}, "status", addon.Namespace, addon)
	f.actions = append(f.actions, action)
}

func getKey(addon *addoncontroller.Addon, t *testing.T) string {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(addon)
	if err != nil {
		t.Errorf("Unexpected error getting key for addon %v: %v", addon.Name, err)
		return ""
	}
	return key
}

func TestCreatesDeployment(t *testing.T) {
	f := newFixture(t)
	addon := newAddon("test", int32Ptr(1))
	_, ctx := ktesting.NewTestContext(t)

	f.addonLister = append(f.addonLister, addon)
	f.objects = append(f.objects, addon)

	expDeployment := newDeployment(addon)
	f.expectCreateDeploymentAction(expDeployment)
	f.expectUpdateAddonStatusAction(addon)

	f.run(ctx, getKey(addon, t))
}

func TestDoNothing(t *testing.T) {
	f := newFixture(t)
	addon := newAddon("test", int32Ptr(1))
	_, ctx := ktesting.NewTestContext(t)

	d := newDeployment(addon)

	f.addonLister = append(f.addonLister, addon)
	f.objects = append(f.objects, addon)
	f.deploymentLister = append(f.deploymentLister, d)
	f.kubeobjects = append(f.kubeobjects, d)

	f.expectUpdateAddonStatusAction(addon)
	f.run(ctx, getKey(addon, t))
}

func TestUpdateDeployment(t *testing.T) {
	f := newFixture(t)
	addon := newAddon("test", int32Ptr(1))
	_, ctx := ktesting.NewTestContext(t)

	d := newDeployment(addon)

	// Update replicas
	addon.Spec.Client.Replicas = int32Ptr(2)
	expDeployment := newDeployment(addon)

	f.addonLister = append(f.addonLister, addon)
	f.objects = append(f.objects, addon)
	f.deploymentLister = append(f.deploymentLister, d)
	f.kubeobjects = append(f.kubeobjects, d)

	f.expectUpdateAddonStatusAction(addon)
	f.expectUpdateDeploymentAction(expDeployment)
	f.run(ctx, getKey(addon, t))
}

func TestNotControlledByUs(t *testing.T) {
	f := newFixture(t)
	addon := newAddon("test", int32Ptr(1))
	_, ctx := ktesting.NewTestContext(t)

	d := newDeployment(addon)

	d.ObjectMeta.OwnerReferences = []metav1.OwnerReference{}

	f.addonLister = append(f.addonLister, addon)
	f.objects = append(f.objects, addon)
	f.deploymentLister = append(f.deploymentLister, d)
	f.kubeobjects = append(f.kubeobjects, d)

	f.runExpectError(ctx, getKey(addon, t))
}

func int32Ptr(i int32) *int32 { return &i }
