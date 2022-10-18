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

// Code generated by informer-gen. DO NOT EDIT.

package v1alpha1

import (
	internalinterfaces "admiralty.io/multicluster-scheduler/pkg/generated/informers/externalversions/internalinterfaces"
)

// Interface provides access to all the informers in this group version.
type Interface interface {
	// ClusterSources returns a ClusterSourceInformer.
	ClusterSources() ClusterSourceInformer
	// ClusterSummaries returns a ClusterSummaryInformer.
	ClusterSummaries() ClusterSummaryInformer
	// ClusterTargets returns a ClusterTargetInformer.
	ClusterTargets() ClusterTargetInformer
	// PodChaperons returns a PodChaperonInformer.
	PodChaperons() PodChaperonInformer
	// Sources returns a SourceInformer.
	Sources() SourceInformer
	// Targets returns a TargetInformer.
	Targets() TargetInformer
}

type version struct {
	factory          internalinterfaces.SharedInformerFactory
	namespace        string
	tweakListOptions internalinterfaces.TweakListOptionsFunc
}

// New returns a new Interface.
func New(f internalinterfaces.SharedInformerFactory, namespace string, tweakListOptions internalinterfaces.TweakListOptionsFunc) Interface {
	return &version{factory: f, namespace: namespace, tweakListOptions: tweakListOptions}
}

// ClusterSources returns a ClusterSourceInformer.
func (v *version) ClusterSources() ClusterSourceInformer {
	return &clusterSourceInformer{factory: v.factory, tweakListOptions: v.tweakListOptions}
}

// ClusterSummaries returns a ClusterSummaryInformer.
func (v *version) ClusterSummaries() ClusterSummaryInformer {
	return &clusterSummaryInformer{factory: v.factory, tweakListOptions: v.tweakListOptions}
}

// ClusterTargets returns a ClusterTargetInformer.
func (v *version) ClusterTargets() ClusterTargetInformer {
	return &clusterTargetInformer{factory: v.factory, tweakListOptions: v.tweakListOptions}
}

// PodChaperons returns a PodChaperonInformer.
func (v *version) PodChaperons() PodChaperonInformer {
	return &podChaperonInformer{factory: v.factory, namespace: v.namespace, tweakListOptions: v.tweakListOptions}
}

// Sources returns a SourceInformer.
func (v *version) Sources() SourceInformer {
	return &sourceInformer{factory: v.factory, namespace: v.namespace, tweakListOptions: v.tweakListOptions}
}

// Targets returns a TargetInformer.
func (v *version) Targets() TargetInformer {
	return &targetInformer{factory: v.factory, namespace: v.namespace, tweakListOptions: v.tweakListOptions}
}