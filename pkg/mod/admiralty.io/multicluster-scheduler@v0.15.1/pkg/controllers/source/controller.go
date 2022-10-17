/*
 * Copyright 2020 The Multicluster-Scheduler Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package source

import (
	"context"
	"os"
	"reflect"
	"time"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	coreinformers "k8s.io/client-go/informers/core/v1"
	rbacinformers "k8s.io/client-go/informers/rbac/v1"
	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	rbaclisters "k8s.io/client-go/listers/rbac/v1"
	"k8s.io/client-go/tools/cache"

	multiclusterv1alpha1 "admiralty.io/multicluster-scheduler/pkg/apis/multicluster/v1alpha1"
	"admiralty.io/multicluster-scheduler/pkg/controller"
	informers "admiralty.io/multicluster-scheduler/pkg/generated/informers/externalversions/multicluster/v1alpha1"
	listers "admiralty.io/multicluster-scheduler/pkg/generated/listers/multicluster/v1alpha1"
	"admiralty.io/multicluster-scheduler/pkg/name"
)

var clusterRoleRefSource = rbacv1.RoleRef{
	APIGroup: "rbac.authorization.k8s.io",
	Kind:     "ClusterRole",
	Name:     os.Getenv("SOURCE_CLUSTER_ROLE_NAME"),
}

var clusterRoleRefClusterSummaryViewer = rbacv1.RoleRef{
	APIGroup: "rbac.authorization.k8s.io",
	Kind:     "ClusterRole",
	Name:     os.Getenv("CLUSTER_SUMMARY_VIEWER_CLUSTER_ROLE_NAME"),
}

type reconciler struct {
	kubeClient kubernetes.Interface

	sourceLister             listers.SourceLister
	clusterSourceLister      listers.ClusterSourceLister
	serviceAccountLister     corelisters.ServiceAccountLister
	roleBindingLister        rbaclisters.RoleBindingLister
	clusterRoleBindingLister rbaclisters.ClusterRoleBindingLister
}

func NewController(
	kubeClient kubernetes.Interface,

	sourceInformer informers.SourceInformer,
	clusterSourceInformer informers.ClusterSourceInformer,
	serviceAccountInformer coreinformers.ServiceAccountInformer,
	roleBindingInformer rbacinformers.RoleBindingInformer,
	clusterRoleBindingInformer rbacinformers.ClusterRoleBindingInformer) *controller.Controller {

	r := &reconciler{
		kubeClient: kubeClient,

		sourceLister:             sourceInformer.Lister(),
		clusterSourceLister:      clusterSourceInformer.Lister(),
		serviceAccountLister:     serviceAccountInformer.Lister(),
		roleBindingLister:        roleBindingInformer.Lister(),
		clusterRoleBindingLister: clusterRoleBindingInformer.Lister(),
	}

	c := controller.New("source", r,
		sourceInformer.Informer().HasSynced,
		clusterSourceInformer.Informer().HasSynced,
		serviceAccountInformer.Informer().HasSynced,
		roleBindingInformer.Informer().HasSynced,
		clusterRoleBindingInformer.Informer().HasSynced)

	sourceInformer.Informer().AddEventHandler(controller.HandleAddUpdateWith(c.EnqueueObject))
	getSource := func(namespace, name string) (metav1.Object, error) {
		return r.sourceLister.Sources(namespace).Get(name)
	}
	serviceAccountInformer.Informer().AddEventHandler(controller.HandleAllWith(c.EnqueueController("Source", getSource)))
	roleBindingInformer.Informer().AddEventHandler(controller.HandleAllWith(c.EnqueueController("Source", getSource)))
	clusterRoleBindingInformer.Informer().AddEventHandler(controller.HandleAllWith(c.EnqueueController("Source", getSource)))

	clusterSourceInformer.Informer().AddEventHandler(controller.HandleAddUpdateWith(c.EnqueueObject))
	getClusterSource := func(namespace, name string) (metav1.Object, error) { return r.clusterSourceLister.Get(name) }
	serviceAccountInformer.Informer().AddEventHandler(controller.HandleAllWith(c.EnqueueController("ClusterSource", getClusterSource)))
	roleBindingInformer.Informer().AddEventHandler(controller.HandleAllWith(c.EnqueueController("ClusterSource", getClusterSource)))
	clusterRoleBindingInformer.Informer().AddEventHandler(controller.HandleAllWith(c.EnqueueController("ClusterSource", getClusterSource)))

	return c
}

func (c *reconciler) Handle(obj interface{}) (requeueAfter *time.Duration, err error) {
	ctx := context.Background()

	key := obj.(string)
	namespace, srcName, err := cache.SplitMetaNamespaceKey(key)
	utilruntime.Must(err)

	var userName, saName, saNamespace, crbName, rbName, clusterSummaryViewerCRBName string
	var ownerRef *metav1.OwnerReference
	if namespace == "" {
		clusterSource, err := c.clusterSourceLister.Get(srcName)
		if err != nil {
			if errors.IsNotFound(err) {
				return nil, nil
			}
			return nil, err
		}
		ownerRef = metav1.NewControllerRef(clusterSource, multiclusterv1alpha1.SchemeGroupVersion.WithKind("ClusterSource"))

		if saRef := clusterSource.Spec.ServiceAccount; saRef != nil {
			saName = saRef.Name
			saNamespace = saRef.Namespace
		}

		userName = clusterSource.Spec.UserName

		crbName = name.FromParts(name.Long, []int{0}, nil,
			"admiralty-cluster-source", clusterSource.Name)
		clusterSummaryViewerCRBName = name.FromParts(name.Long, []int{0, 2}, nil,
			"admiralty-cluster-source", clusterSource.Name, "cluster-summary-viewer")
	} else {
		source, err := c.sourceLister.Sources(namespace).Get(srcName)
		if err != nil {
			if errors.IsNotFound(err) {
				return nil, nil
			}
			return nil, err
		}
		ownerRef = metav1.NewControllerRef(source, multiclusterv1alpha1.SchemeGroupVersion.WithKind("Source"))

		saName = source.Spec.ServiceAccountName
		saNamespace = source.Namespace

		userName = source.Spec.UserName

		rbName = name.FromParts(name.Long, []int{0}, nil,
			"admiralty-source", source.Name)
		clusterSummaryViewerCRBName = name.FromParts(name.Long, []int{0, 3}, nil,
			"admiralty-source", source.Namespace, source.Name, "cluster-summary-viewer")
	}

	if saName != "" {
		_, err := c.serviceAccountLister.ServiceAccounts(saNamespace).Get(saName)
		if err != nil {
			if !errors.IsNotFound(err) {
				return nil, err
			}

			gold := &corev1.ServiceAccount{}
			gold.Name = saName
			gold.OwnerReferences = []metav1.OwnerReference{*ownerRef}
			_, err = c.kubeClient.CoreV1().ServiceAccounts(namespace).Create(ctx, gold, metav1.CreateOptions{})
			if err != nil {
				return nil, err
			}
		}
	}

	if userName != "" || saName != "" {
		if namespace == "" {
			requeueAfter, err := c.ensureClusterRoleBinding(ctx, crbName, clusterRoleRefSource, userName, saName, saNamespace, ownerRef)
			if requeueAfter != nil || err != nil {
				return requeueAfter, err
			}
		} else {
			requeueAfter, err := c.ensureRoleBinding(ctx, rbName, namespace, clusterRoleRefSource, userName, saName, saNamespace, ownerRef)
			if requeueAfter != nil || err != nil {
				return requeueAfter, err
			}
		}

		requeueAfter, err := c.ensureClusterRoleBinding(ctx, clusterSummaryViewerCRBName,
			clusterRoleRefClusterSummaryViewer, userName, saName, saNamespace, ownerRef)
		if requeueAfter != nil || err != nil {
			return requeueAfter, err
		}
	}

	return nil, nil
}

func (c *reconciler) ensureClusterRoleBinding(ctx context.Context, name string, roleRef rbacv1.RoleRef, userName, saName, saNamespace string, ownerRef *metav1.OwnerReference) (requeueAfter *time.Duration, err error) {
	subjects := makeSubjects(saName, saNamespace, userName)

	crb, err := c.clusterRoleBindingLister.Get(name)
	if err != nil {
		if !errors.IsNotFound(err) {
			return nil, err
		}

		gold := &rbacv1.ClusterRoleBinding{}
		gold.Name = name
		gold.OwnerReferences = []metav1.OwnerReference{*ownerRef}
		gold.Subjects = makeSubjects(saName, saNamespace, userName)
		gold.RoleRef = roleRef
		crb, err = c.kubeClient.RbacV1().ClusterRoleBindings().Create(ctx, gold, metav1.CreateOptions{})
		if err != nil {
			return nil, err
		}
	} else if !reflect.DeepEqual(crb.Subjects, subjects) {
		actualCopy := crb.DeepCopy()
		actualCopy.Subjects = subjects
		crb, err = c.kubeClient.RbacV1().ClusterRoleBindings().Update(ctx, actualCopy, metav1.UpdateOptions{})
		if err != nil {
			return nil, err
		}
	}

	return nil, nil
}

func (c *reconciler) ensureRoleBinding(ctx context.Context, name, namespace string, roleRef rbacv1.RoleRef, userName, saName, saNamespace string, ownerRef *metav1.OwnerReference) (requeueAfter *time.Duration, err error) {
	subjects := makeSubjects(saName, saNamespace, userName)

	rb, err := c.roleBindingLister.RoleBindings(namespace).Get(name)
	if err != nil {
		if !errors.IsNotFound(err) {
			return nil, err
		}

		gold := &rbacv1.RoleBinding{}
		gold.Name = name
		gold.OwnerReferences = []metav1.OwnerReference{*ownerRef}
		gold.Subjects = makeSubjects(saName, saNamespace, userName)
		gold.RoleRef = clusterRoleRefSource
		rb, err = c.kubeClient.RbacV1().RoleBindings(namespace).Create(ctx, gold, metav1.CreateOptions{})
		if err != nil {
			return nil, err
		}
	} else if !reflect.DeepEqual(rb.Subjects, subjects) {
		actualCopy := rb.DeepCopy()
		actualCopy.Subjects = subjects
		rb, err = c.kubeClient.RbacV1().RoleBindings(namespace).Update(ctx, actualCopy, metav1.UpdateOptions{})
		if err != nil {
			return nil, err
		}
	}

	return nil, nil
}

func makeSubjects(saName string, saNamespace string, userName string) []rbacv1.Subject {
	var subjects []rbacv1.Subject
	if saName != "" {
		subjects = append(subjects, rbacv1.Subject{
			Kind:      "ServiceAccount",
			Name:      saName,
			Namespace: saNamespace,
		})
	}
	if userName != "" {
		subjects = append(subjects, rbacv1.Subject{
			Kind: "User",
			Name: userName,
		})
	}
	return subjects
}
