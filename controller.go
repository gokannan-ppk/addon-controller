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
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	appsinformers "k8s.io/client-go/informers/apps/v1"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	appslisters "k8s.io/client-go/listers/apps/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	samplev1alpha1 "github.com/gokannan-ppk/addon-controller/pkg/apis/addoncontroller/v1alpha1"
	clientset "github.com/gokannan-ppk/addon-controller/pkg/generated/clientset/versioned"
	samplescheme "github.com/gokannan-ppk/addon-controller/pkg/generated/clientset/versioned/scheme"
	informers "github.com/gokannan-ppk/addon-controller/pkg/generated/informers/externalversions/addoncontroller/v1alpha1"
	listers "github.com/gokannan-ppk/addon-controller/pkg/generated/listers/addoncontroller/v1alpha1"
)

const controllerAgentName = "addon-controller"

const (
	// SuccessSynced is used as part of the Event 'reason' when a Addon is synced
	SuccessSynced = "Synced"
	// ErrResourceExists is used as part of the Event 'reason' when a Addon fails
	// to sync due to a Deployment of the same name already existing.
	ErrResourceExists = "ErrResourceExists"

	// MessageResourceExists is the message used for Events when a resource
	// fails to sync due to a Deployment already existing
	MessageResourceExists = "Resource %q already exists and is not managed by Addon"
	// MessageResourceSynced is the message used for an Event fired when a Addon
	// is synced successfully
	MessageResourceSynced = "Addon synced successfully"
)

// Controller is the controller implementation for Addon resources
type Controller struct {
	// kubeclientset is a standard kubernetes clientset
	kubeclientset kubernetes.Interface
	// addonclientset is a clientset for our own API group
	addonclientset clientset.Interface

	deploymentsLister appslisters.DeploymentLister
	deploymentsSynced cache.InformerSynced
	servicesLister    corelisters.ServiceLister
	servicesSynced    cache.InformerSynced
	configmapsLister  corelisters.ConfigMapLister
	addonsLister      listers.AddonLister
	addonsSynced      cache.InformerSynced

	// workqueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	workqueue workqueue.RateLimitingInterface
	// recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	recorder record.EventRecorder
}

// NewController returns a new sample controller
func NewController(
	ctx context.Context,
	kubeclientset kubernetes.Interface,
	addonclientset clientset.Interface,
	deploymentInformer appsinformers.DeploymentInformer,
	serviceInformer coreinformers.ServiceInformer,
	configmapInformer coreinformers.ConfigMapInformer,
	addonInformer informers.AddonInformer) *Controller {
	logger := klog.FromContext(ctx)

	// Create event broadcaster
	// Add addon-controller types to the default Kubernetes Scheme so Events can be
	// logged for addon-controller types.
	utilruntime.Must(samplescheme.AddToScheme(scheme.Scheme))
	logger.V(4).Info("Creating event broadcaster")

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartStructuredLogging(0)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	controller := &Controller{
		kubeclientset:     kubeclientset,
		addonclientset:    addonclientset,
		deploymentsLister: deploymentInformer.Lister(),
		deploymentsSynced: deploymentInformer.Informer().HasSynced,
		servicesLister:    serviceInformer.Lister(),
		servicesSynced:    serviceInformer.Informer().HasSynced,
		configmapsLister:  configmapInformer.Lister(),
		addonsLister:      addonInformer.Lister(),
		addonsSynced:      addonInformer.Informer().HasSynced,
		workqueue:         workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Addons"),
		recorder:          recorder,
	}

	logger.Info("Setting up event handlers")
	// Set up an event handler for when Addon resources change
	addonInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueueAddon,
		UpdateFunc: func(old, new interface{}) {
			controller.enqueueAddon(new)
		},
		DeleteFunc: controller.deleteAddon,
	})
	deploymentInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.handleObject,
		UpdateFunc: func(old, new interface{}) {
			newDepl := new.(*appsv1.Deployment)
			oldDepl := old.(*appsv1.Deployment)
			if newDepl.ResourceVersion == oldDepl.ResourceVersion {
				return
			}
			controller.handleObject(new)
		},
		DeleteFunc: controller.handleObject,
	})
	serviceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.handleObject,
		UpdateFunc: func(old, new interface{}) {
			newDepl := new.(*corev1.Service)
			oldDepl := old.(*corev1.Service)
			if newDepl.ResourceVersion == oldDepl.ResourceVersion {
				return
			}
			controller.handleObject(new)
		},
		DeleteFunc: controller.handleObject,
	})

	return controller
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
func (c *Controller) Run(ctx context.Context, workers int) error {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()
	logger := klog.FromContext(ctx)

	// Start the informer factories to begin populating the informer caches
	logger.Info("Starting Addon controller")

	// Wait for the caches to be synced before starting workers
	logger.Info("Waiting for informer caches to sync")

	// configmap的增删改不需要感知，默认环境中就是有相应的环境配置的，使用时直接查就行
	if ok := cache.WaitForCacheSync(ctx.Done(), c.deploymentsSynced, c.servicesSynced, c.addonsSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	logger.Info("Starting workers", "count", workers)
	// Launch two workers to process Addon resources
	for i := 0; i < workers; i++ {
		go wait.UntilWithContext(ctx, c.runWorker, time.Second)
	}

	logger.Info("Started workers")
	<-ctx.Done()
	logger.Info("Shutting down workers")

	return nil
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (c *Controller) runWorker(ctx context.Context) {
	for c.processNextWorkItem(ctx) {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (c *Controller) processNextWorkItem(ctx context.Context) bool {
	obj, shutdown := c.workqueue.Get()
	logger := klog.FromContext(ctx)

	if shutdown {
		return false
	}

	// We wrap this block in a func so we can defer c.workqueue.Done.
	err := func(obj interface{}) error {
		// We call Done here so the workqueue knows we have finished
		// processing this item. We also must remember to call Forget if we
		// do not want this work item being re-queued. For example, we do
		// not call Forget if a transient error occurs, instead the item is
		// put back on the workqueue and attempted again after a back-off
		// period.
		defer c.workqueue.Done(obj)
		var key string
		var ok bool
		// We expect strings to come off the workqueue. These are of the
		// form namespace/name. We do this as the delayed nature of the
		// workqueue means the items in the informer cache may actually be
		// more up to date that when the item was initially put onto the
		// workqueue.
		if key, ok = obj.(string); !ok {
			// As the item in the workqueue is actually invalid, we call
			// Forget here else we'd go into a loop of attempting to
			// process a work item that is invalid.
			c.workqueue.Forget(obj)
			utilruntime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}
		// Run the syncHandler, passing it the namespace/name string of the
		// Addon resource to be synced.
		if err := c.syncHandler(ctx, key); err != nil {
			// Put the item back on the workqueue to handle any transient errors.
			c.workqueue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.workqueue.Forget(obj)
		logger.Info("Successfully synced", "resourceName", key)
		return nil
	}(obj)

	if err != nil {
		utilruntime.HandleError(err)
		return true
	}

	return true
}

// syncHandler compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the Addon resource
// with the current status of the resource.
func (c *Controller) syncHandler(ctx context.Context, key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	logger := klog.LoggerWithValues(klog.FromContext(ctx), "resourceName", key)

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	// Get the Addon resource with this namespace/name
	addon, err := c.addonsLister.Addons(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			utilruntime.HandleError(fmt.Errorf("addon '%s' in work queue no longer exists", key))
			return nil
		}
		return err
	}

	// 检查DeletionTimestamp字段是否为0来判断资源是否被删除
	if addon.ObjectMeta.DeletionTimestamp.IsZero() {
		// 如果Addon对象未被删除，则检测finalizer是否存在，若不存在，则添加更新到Addon资源对象中
		if !finalizerContains(addon.ObjectMeta.Finalizers, samplev1alpha1.FinalizerNameHelmClient) {
			addon.ObjectMeta.Finalizers = append(addon.ObjectMeta.Finalizers, samplev1alpha1.FinalizerNameHelmClient)
		}
		if !finalizerContains(addon.ObjectMeta.Finalizers, samplev1alpha1.FinalizerNameHelmChart) {
			addon.ObjectMeta.Finalizers = append(addon.ObjectMeta.Finalizers, samplev1alpha1.FinalizerNameHelmChart)
		}
		if !finalizerContains(addon.ObjectMeta.Finalizers, samplev1alpha1.FinalizerNameCCEInfo) {
			addon.ObjectMeta.Finalizers = append(addon.ObjectMeta.Finalizers, samplev1alpha1.FinalizerNameCCEInfo)
		}
		if _, err := c.addonclientset.AddoncontrollerV1alpha1().Addons(addon.Namespace).Update(context.TODO(), addon, metav1.UpdateOptions{}); err != nil {
			return err
		}
	} else {
		// 如果Addon对象处于删除中，则要遍历finalizers，然后针对每一种级联资源进行删除操作
		for _, finalizer := range addon.ObjectMeta.Finalizers {
			// todo：实现3种级联资源的pre delete hook逻辑，注意要先删除CCE中的注册信息
			if err := c.deleteExternalResources(addon); err != nil {
				// 如果删除失败，则直接返回err，controller需要重新做入队处理
				return err
			}
		}
		return nil
	}

	// 依次处理3种级联资源：1）自定义helm客户端，供controller调用接口上传helm chart；2）helm chart；3）CCE中的注册信息。
	// 1）自定义helm客户端，使用deployment创建，同时创建service
	// 从clientSpec定义中获取deployment name
	clientName := addon.Spec.Client.Name
	if clientName == "" {
		utilruntime.HandleError(fmt.Errorf("%s: deployment name must be specified", key))
		return nil
	}

	deployment, err := c.deploymentsLister.Deployments(addon.Namespace).Get(clientName)
	if errors.IsNotFound(err) {
		deployment, err = c.kubeclientset.AppsV1().Deployments(addon.Namespace).Create(context.TODO(), newDeployment(addon), metav1.CreateOptions{})
	}
	if err != nil {
		return err
	}
	if !metav1.IsControlledBy(deployment, addon) {
		msg := fmt.Sprintf(MessageResourceExists, deployment.Name)
		c.recorder.Event(addon, corev1.EventTypeWarning, ErrResourceExists, msg)
		return fmt.Errorf("%s", msg)
	}
	if addon.Spec.Client.Replicas != nil && *addon.Spec.Client.Replicas != *deployment.Spec.Replicas {
		logger.V(4).Info("Update deployment resource", "currentReplicas", *addon.Spec.Client.Replicas, "desiredReplicas", *deployment.Spec.Replicas)
		deployment, err = c.kubeclientset.AppsV1().Deployments(addon.Namespace).Update(context.TODO(), newDeployment(addon), metav1.UpdateOptions{})
	}
	if err != nil {
		return err
	}

	// 获取service，如果没有则创建
	service, err := c.servicesLister.Services(addon.Namespace).Get(clientName)
	if errors.IsNotFound(err) {
		service, err = c.kubeclientset.CoreV1().Services(addon.Namespace).Create(context.TODO(), newService(addon), metav1.CreateOptions{})
	}
	if err != nil {
		return err
	}
	if !metav1.IsControlledBy(service, addon) {
		msg := fmt.Sprintf(MessageResourceExists, service.Name)
		c.recorder.Event(addon, corev1.EventTypeWarning, ErrResourceExists, msg)
		return fmt.Errorf("%s", msg)
	}

	// 2）helm chart上传
	// todo：从环境（controller所在k8s集群）中获取环境配置，包含helm仓库地址、镜像仓库地址、cce服务地址等
	// todo：通过上面用deployment创建的客户端调用helm chart上传接口上传chart

	// 3）注册Addon版本信息到CCE服务
	// todo：通过从配置中获取到的CCE服务信息，调Addon版本注册接口，一旦注册成功，用户就能看到并使用相应的插件版本

	// 更新Addon状态并记录事件
	err = c.updateAddonStatus(addon, deployment)
	if err != nil {
		return err
	}
	c.recorder.Event(addon, corev1.EventTypeNormal, SuccessSynced, MessageResourceSynced)
	return nil
}

func finalizerContains(strs []string, s string) bool {
	for _, str := range strs {
		if str == s {
			return true
		}
	}
	return false
}

// todo：实现删除级联资源的具体逻辑
func (c *Controller) deleteExternalResources(addon *samplev1alpha1.Addon) error {
	// 删除Addon关联的外部资源逻辑
	// 确保实现是幂等的
	return nil
}

func (c *Controller) updateAddonStatus(addon *samplev1alpha1.Addon, deployment *appsv1.Deployment) error {
	// todo：需要定义addon的status后再补充相关逻辑
	return nil
}

func (c *Controller) enqueueAddon(obj interface{}) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.workqueue.Add(key)
}

// Addon资源删除的响应方法，待实现
func (c *Controller) deleteAddon(obj interface{}) {
	//todo：具体实现逻辑
}

func (c *Controller) handleObject(obj interface{}) {
	var object metav1.Object
	var ok bool
	logger := klog.FromContext(context.Background())
	if object, ok = obj.(metav1.Object); !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding object, invalid type"))
			return
		}
		object, ok = tombstone.Obj.(metav1.Object)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding object tombstone, invalid type"))
			return
		}
		logger.V(4).Info("Recovered deleted object", "resourceName", object.GetName())
	}
	logger.V(4).Info("Processing object", "object", klog.KObj(object))
	if ownerRef := metav1.GetControllerOf(object); ownerRef != nil {
		// If this object is not owned by a Addon, we should not do anything more
		// with it.
		if ownerRef.Kind != "Addon" {
			return
		}

		addon, err := c.addonsLister.Addons(object.GetNamespace()).Get(ownerRef.Name)
		if err != nil {
			logger.V(4).Info("Ignore orphaned object", "object", klog.KObj(object), "addon", ownerRef.Name)
			return
		}

		c.enqueueAddon(addon)
		return
	}
}

func newDeployment(addon *samplev1alpha1.Addon) *appsv1.Deployment {
	labels := map[string]string{
		"app":        "repo-client",
		"controller": addon.Name,
	}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      addon.Spec.Client.Name,
			Namespace: addon.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(addon, samplev1alpha1.SchemeGroupVersion.WithKind("Addon")),
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: addon.Spec.Client.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  addon.Spec.Client.Name,
							Image: addon.Spec.Client.Image,
						},
					},
				},
			},
		},
	}
}

// 注意添加OwnerReferences
func newService(addon *samplev1alpha1.Addon) *corev1.Service {
	labels := map[string]string{
		"app":        "repo-client",
		"controller": addon.Name,
	}
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      addon.Spec.Client.Name,
			Namespace: addon.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(addon, samplev1alpha1.SchemeGroupVersion.WithKind("Addon")),
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Protocol:   corev1.ProtocolTCP,
					Port:       80,
					TargetPort: intstr.IntOrString{IntVal: 80},
				},
			},
		},
	}
}
