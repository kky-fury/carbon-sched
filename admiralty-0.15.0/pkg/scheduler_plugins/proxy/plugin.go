/*
 * Copyright 2022 The Multicluster-Scheduler Authors.
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

package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	"admiralty.io/multicluster-scheduler/pkg/apis/multicluster/v1alpha1"
	"admiralty.io/multicluster-scheduler/pkg/common"
	agentconfig "admiralty.io/multicluster-scheduler/pkg/config/agent"
	"admiralty.io/multicluster-scheduler/pkg/generated/clientset/versioned"
	"admiralty.io/multicluster-scheduler/pkg/model/delegatepod"
)

type Plugin struct {
	handle           framework.Handle
	targets          map[string]*versioned.Clientset
	targetNamespaces map[string]string

	failedNodeNamesByPodUID map[types.UID]map[string]bool
	mx                      sync.RWMutex
}

var _ framework.FilterPlugin = &Plugin{}
var _ framework.PostFilterPlugin = &Plugin{}
var _ framework.ReservePlugin = &Plugin{}
var _ framework.PreBindPlugin = &Plugin{}
var _ framework.PostBindPlugin = &Plugin{}
var _ framework.ScorePlugin = &Plugin{}

var EmissionRank = map[string]int64{}
var refreshTimer *time.Timer

// Name is the name of the plugin used in the plugin registry and configurations.
const Name = "proxy"

// Name returns name of the plugin. It is used in logs, etc.
func (pl *Plugin) Name() string {
	return Name
}

func virtualNodeNameToClusterName(nodeName string) string {
	return nodeName
}

func (pl *Plugin) Score(ctx context.Context, state *framework.CycleState, p *v1.Pod, nodeName string) (int64, *framework.Status) {
	nodeInfo, err := pl.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)
	if err != nil {
		return 0, framework.AsStatus(fmt.Errorf("getting node %q from Snapshot: %w", nodeName, err))
	}

	//nodeInfos, err := pl.handle.SnapshotSharedLister().NodeInfos().List()
	//if err != nil {
	//	return 0, framework.AsStatus(err)
	//}
	//totalNumNodes := len(nodeInfos)

	score := calculateScores(nodeInfo, refreshTimer)
	klog.Infof("debug point")
	// TODO: Implement carbon-aware scoring
	return score, nil
}

func (pl *Plugin) ScoreExtensions() framework.ScoreExtensions {
	// TODO: Implement carbon-aware scoring
	return nil
}

func calculateScores(nodeInfo *framework.NodeInfo, refreshTimer *time.Timer) int64 {
	if refreshTimer == nil {
		refreshTimer = time.NewTimer(5 * time.Minute)
	}
	emissionRank, err := getEmissionRanking(refreshTimer)
	if err != nil {
		klog.Info(err)
		return 0
	}
	//region := nodeInfo.Node().Labels["node.kubernetes.io/region"]
	region := nodeInfo.Node().Annotations["node.kubernetes.io/region"]
	//score := emissionRank[region]
	score := 7
	if score == 0 {
		return 0
	}
	klog.InfoS("Populated map: ", emissionRank)
	klog.InfoS("Populated map1: ", EmissionRank)
	klog.Infof("Score calculated: ", region, " Score:", 10*score)
	return int64(10 * score)
}

func getEmissionRanking(refreshTimer *time.Timer) (map[string]int64, error) {
	if len(EmissionRank) == 0 {
		err := queryDataFromServer()
		if err != nil {
			return map[string]int64{}, err
		}
		return EmissionRank, nil
	}
	select {
	case <-refreshTimer.C:
		err := queryDataFromServer()
		if err != nil {
			return map[string]int64{}, err
		}
		return EmissionRank, nil
	default:
		return EmissionRank, nil
	}
}

func queryDataFromServer() error {
	url := "http://metrics-collector.default.svc.cluster.local:8080/getemission"
	method := "GET"
	client := &http.Client{}

	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		klog.Info(err)
		return err
	}

	res, err := client.Do(req)
	if err != nil {
		klog.Info(err)
		return err
	}
	decoder := json.NewDecoder(res.Body)
	err = decoder.Decode(&EmissionRank)
	if err != nil {
		klog.Info(err)
		return err
	}
	return nil
}

func (pl *Plugin) getCandidate(ctx context.Context, proxyPod *v1.Pod, clusterName string) (*v1alpha1.PodChaperon, error) {
	target, ok := pl.targets[clusterName]
	if !ok {
		return nil, fmt.Errorf("no target for cluster name %s", clusterName)
	}
	l, err := target.MulticlusterV1alpha1().PodChaperons(proxyPod.Namespace).List(ctx, metav1.ListOptions{LabelSelector: common.LabelKeyParentUID + "=" + string(proxyPod.UID)})
	if err != nil {
		return nil, err
	}
	if len(l.Items) > 1 {
		return nil, fmt.Errorf("more than one candidate in target cluster")
	}
	if len(l.Items) < 1 {
		return nil, nil
	}
	return &l.Items[0], nil
}

func (pl *Plugin) allowCandidate(ctx context.Context, c *v1alpha1.PodChaperon, clusterName string) error {
	target, ok := pl.targets[clusterName]
	if !ok {
		return fmt.Errorf("no target for cluster name %s", clusterName)
	}
	patch := []byte(`{"metadata":{"annotations":{"` + common.AnnotationKeyIsAllowed + `":"true"}}}`)
	_, err := target.MulticlusterV1alpha1().PodChaperons(c.Namespace).Patch(ctx, c.Name, types.MergePatchType, patch, metav1.PatchOptions{})
	return err
}

const filterWaitDuration = 30 * time.Second // TODO configure

func (pl *Plugin) Filter(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	if nodeInfo.Node().Labels[common.LabelAndTaintKeyVirtualKubeletProvider] != common.VirtualKubeletProviderName {
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, "")
	}

	if pl.unreservedInAPreviousCycle(pod.UID, nodeInfo.Node().Name) {
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, "unreserved in a previous cycle")
	}

	if ns := nodeInfo.Node().Labels[common.LabelKeyTargetNamespace]; ns != "" && ns != pod.Namespace {
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, "target in different namespace")
	}

	// working without a candidate scheduler, we'll create a single candidate AFTER a virtual node is selected
	if _, ok := pod.Annotations[common.AnnotationKeyNoReservation]; ok {
		return nil
	}

	targetClusterName := virtualNodeNameToClusterName(nodeInfo.Node().Name)

	ctx, cancel := context.WithTimeout(ctx, filterWaitDuration)
	defer cancel()

	var isReserved, isUnschedulable bool

	if err := wait.PollImmediateUntil(time.Second, func() (bool, error) {
		c, err := pl.getCandidate(ctx, pod, targetClusterName)
		if err != nil {
			// may be forbidden, or namespace doesn't exist, or target cluster is unavailable
			// handled below as unschedulable
			return false, err
		}
		// create candidate if not exists
		if c == nil {
			c, err := delegatepod.MakeDelegatePod(pod)
			if err != nil {
				return false, err
			}

			_, err = pl.targets[targetClusterName].MulticlusterV1alpha1().PodChaperons(c.Namespace).Create(ctx, c, metav1.CreateOptions{})
			if err != nil {
				// may be forbidden, or namespace doesn't exist, or target cluster is unavailable
				// handled below as unschedulable
				return false, err
			}

			return false, nil
		}
		_, isReserved = c.Annotations[common.AnnotationKeyIsReserved]

		for _, cond := range c.Status.Conditions {
			if cond.Type == v1.PodScheduled && cond.Status == v1.ConditionFalse && cond.Reason == v1.PodReasonUnschedulable {
				isUnschedulable = true
				break
			}
		}

		klog.V(1).Infof("candidate %s is reserved? %v unschedulable? %v", c.Name, isReserved, isUnschedulable)

		return isReserved || isUnschedulable, nil
	}, ctx.Done()); err != nil {
		// error or timeout or scheduling cycle done
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, err.Error())
	}

	if isUnschedulable {
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, "")
	}

	return nil
}

func (pl *Plugin) unreservedInAPreviousCycle(podUID types.UID, nodeName string) bool {
	pl.mx.RLock()
	defer pl.mx.RUnlock()
	return pl.failedNodeNamesByPodUID[podUID][nodeName]
}

func (pl *Plugin) PostFilter(ctx context.Context, state *framework.CycleState, pod *v1.Pod, filteredNodeStatusMap framework.NodeToStatusMap) (*framework.PostFilterResult, *framework.Status) {
	pl.failedNodeNamesByPodUID[pod.UID] = nil
	return nil, framework.NewStatus(framework.Unschedulable)
}

func (pl *Plugin) Reserve(ctx context.Context, state *framework.CycleState, p *v1.Pod, nodeName string) *framework.Status {
	targetClusterName := virtualNodeNameToClusterName(nodeName)
	c, err := pl.getCandidate(ctx, p, targetClusterName)
	if err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	}
	if c == nil {
		if _, ok := p.Annotations[common.AnnotationKeyNoReservation]; ok {
			c, err := delegatepod.MakeDelegatePod(p)
			if err != nil {
				return framework.NewStatus(framework.Error, err.Error())
			}

			_, err = pl.targets[targetClusterName].MulticlusterV1alpha1().PodChaperons(c.Namespace).Create(ctx, c, metav1.CreateOptions{})
			if err != nil {
				// may be forbidden, or namespace doesn't exist, or target cluster is unavailable
				return framework.NewStatus(framework.Error, err.Error())
			}

			return nil
		}
		return framework.NewStatus(framework.Error, "candidate not found")
	}
	if err = pl.allowCandidate(ctx, c, targetClusterName); err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	}

	return nil
}

func (pl *Plugin) Unreserve(ctx context.Context, state *framework.CycleState, p *v1.Pod, nodeName string) {
	// remember that this virtual node should be filtered out in next scheduling cycle (a.k.a. "filter a posteriori")
	// unless we've filtered out all other nodes already
	// in which case we reset the memory and try again to see if things have changed
	pl.mx.Lock()
	defer pl.mx.Unlock()

	if pl.failedNodeNamesByPodUID[p.UID] == nil {
		pl.failedNodeNamesByPodUID[p.UID] = map[string]bool{}
	}
	pl.failedNodeNamesByPodUID[p.UID][nodeName] = true
}

const preBindWaitDuration = 60 * time.Second // increased from arbitrary 30 seconds, because Fargate takes 30-60 seconds

func (pl *Plugin) PreBind(ctx context.Context, state *framework.CycleState, p *v1.Pod, nodeName string) *framework.Status {
	// wait for candidate to be bound or not
	targetClusterName := virtualNodeNameToClusterName(nodeName)

	ctx, cancel := context.WithTimeout(ctx, preBindWaitDuration)
	defer cancel()

	// TODO subscribe to a controller instead of polling
	if err := wait.PollImmediateUntil(time.Second, func() (bool, error) {
		return pl.candidateIsBound(ctx, p, targetClusterName)
	}, ctx.Done()); err != nil {
		// or binding cycle done, candidate was never bound or not
		return framework.NewStatus(framework.Error, err.Error())
	}

	return nil
}

func (pl *Plugin) candidateIsBound(ctx context.Context, p *v1.Pod, targetClusterName string) (bool, error) {
	c, err := pl.getCandidate(ctx, p, targetClusterName)
	if err != nil {
		// TODO handle retriable vs. not retriable (we assume retriable for now)
		// TODO log
		return false, nil
	}
	if c == nil {
		return false, fmt.Errorf("candidate not found")
	}

	for _, cond := range c.Status.Conditions {
		if cond.Type == v1.PodScheduled {
			if cond.Status == v1.ConditionTrue { // bound
				return true, nil
			} else { // binding failed
				return false, fmt.Errorf("candidate binding failed")
			}
		}
	}
	return false, nil
}

func (pl *Plugin) PostBind(ctx context.Context, state *framework.CycleState, p *v1.Pod, nodeName string) {
	targetClusterName := virtualNodeNameToClusterName(nodeName)
	for clusterName, target := range pl.targets {
		if clusterName == targetClusterName {
			continue
		}
		if ns := pl.targetNamespaces[clusterName]; ns != "" && ns != p.Namespace {
			continue
		}
		err := target.MulticlusterV1alpha1().PodChaperons(p.Namespace).DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{LabelSelector: common.LabelKeyParentUID + "=" + string(p.UID)})
		utilruntime.HandleError(err)
	}

	pl.mx.Lock()
	defer pl.mx.Unlock()
	delete(pl.failedNodeNamesByPodUID, p.UID)
	// TODO if a proxy pod is deleted while pending, with failed node names, PostBind won't be called,
	// so we're leaking memory, but there's no multi-cycle "FinalUnreserve" plugin, we'd have to listen to deletions...
}

// New initializes a new plugin and returns it.
func New(_ runtime.Object, h framework.Handle) (framework.Plugin, error) {
	agentCfg := agentconfig.NewFromCRD(context.Background())
	n := len(agentCfg.Targets)
	targets := make(map[string]*versioned.Clientset, n)
	targetNamespaces := make(map[string]string, n)
	for _, target := range agentCfg.Targets {
		client, err := versioned.NewForConfig(target.ClientConfig)
		utilruntime.Must(err)
		targets[target.VirtualNodeName] = client
		targetNamespaces[target.VirtualNodeName] = target.Namespace
	}
	// TODO... cache podchaperons with lister

	return &Plugin{
		handle:                  h,
		targets:                 targets,
		targetNamespaces:        targetNamespaces,
		failedNodeNamesByPodUID: map[types.UID]map[string]bool{},
	}, nil
}
