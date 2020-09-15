/*
Copyright 2019 Bloomberg Finance LP.

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

package util

import (
	solr "github.com/bloomberg/solr-operator/api/v1beta1"
	"github.com/bloomberg/solr-operator/controllers/util/solr_api"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"testing"
)

var (
	expectedBackupRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo-back", Namespace: "default"}}
)

func init() {
}

func TestPickPodsToUpgrade(t *testing.T) {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	overseerLeader := "pod-0.foo-solrcloud-headless.default:2000_solr"

	maxshardReplicasUnavailable := intstr.FromInt(1)

	solrCloud := &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
		Spec: solr.SolrCloudSpec{
			SolrAddressability: solr.SolrAddressabilityOptions{
				PodPort: 2000,
			},
			UpdateStrategy: solr.SolrUpdateStrategy{
				Method: solr.ManagedUpdate,
				ManagedUpdateOptions: solr.ManagedUpdateOptions{
					MaxShardReplicasUnavailable: &maxshardReplicasUnavailable,
				},
			},
		},
	}

	allPods := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-0"}, Spec: corev1.PodSpec{}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-1"}, Spec: corev1.PodSpec{}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-2"}, Spec: corev1.PodSpec{}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-3"}, Spec: corev1.PodSpec{}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-4"}, Spec: corev1.PodSpec{}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-5"}, Spec: corev1.PodSpec{}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-6"}, Spec: corev1.PodSpec{}},
	}

	halfPods := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-0"}, Spec: corev1.PodSpec{}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-1"}, Spec: corev1.PodSpec{}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-3"}, Spec: corev1.PodSpec{}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-5"}, Spec: corev1.PodSpec{}},
	}

	lastPod := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-0"}, Spec: corev1.PodSpec{}},
	}

	/*
		Test upgrades with down replicas (which do not affect what nodes can be upgraded)
	*/

	// Normal inputs
	maxshardReplicasUnavailable = intstr.FromInt(1)
	podsToUpgrade := getPodNames(pickPodsToUpgrade(solrCloud, allPods, testDownClusterStatus, overseerLeader, 6, 6))
	assert.ElementsMatch(t, []string{"pod-2", "pod-6"}, podsToUpgrade, "Incorrect set of next pods to upgrade. Do to the down/non-live replicas, only the node without replicas and one more can be upgraded.")

	// Test the maxBatchNodeUpgradeSpec
	maxshardReplicasUnavailable = intstr.FromInt(1)
	podsToUpgrade = getPodNames(pickPodsToUpgrade(solrCloud, allPods, testDownClusterStatus, overseerLeader, 6, 1))
	assert.ElementsMatch(t, []string{"pod-6"}, podsToUpgrade, "Incorrect set of next pods to upgrade. Only 1 node should be upgraded when maxBatchNodeUpgradeSpec=1")

	// Test the maxShardReplicasDownSpec
	maxshardReplicasUnavailable = intstr.FromInt(2)
	podsToUpgrade = getPodNames(pickPodsToUpgrade(solrCloud, allPods, testDownClusterStatus, overseerLeader, 6, 6))
	assert.ElementsMatch(t, []string{"pod-2", "pod-3", "pod-4", "pod-6"}, podsToUpgrade, "Incorrect set of next pods to upgrade.")

	/*
		Test upgrades with replicas in recovery (which are treated as "active" when calculating how many nodes can be taken down) and a non-live node.
	*/

	// Normal inputs
	maxshardReplicasUnavailable = intstr.FromInt(1)
	podsToUpgrade = getPodNames(pickPodsToUpgrade(solrCloud, allPods, testRecoveringClusterStatus, overseerLeader, 6, 6))
	assert.ElementsMatch(t, []string{"pod-4", "pod-6"}, podsToUpgrade, "Incorrect set of next pods to upgrade. Do to the recovering/down/non-live replicas, only the non-live node and node without replicas can be upgraded.")

	// Test the maxBatchNodeUpgradeSpec
	maxshardReplicasUnavailable = intstr.FromInt(1)
	podsToUpgrade = getPodNames(pickPodsToUpgrade(solrCloud, allPods, testRecoveringClusterStatus, overseerLeader, 6, 1))
	assert.ElementsMatch(t, []string{"pod-6"}, podsToUpgrade, "Incorrect set of next pods to upgrade. Only 1 node should be upgraded when maxBatchNodeUpgradeSpec=1")

	// Test the maxShardReplicasDownSpec
	maxshardReplicasUnavailable = intstr.FromInt(2)
	podsToUpgrade = getPodNames(pickPodsToUpgrade(solrCloud, allPods, testRecoveringClusterStatus, overseerLeader, 6, 6))
	assert.ElementsMatch(t, []string{"pod-2", "pod-3", "pod-4", "pod-6"}, podsToUpgrade, "Incorrect set of next pods to upgrade. More nodes should be upgraded when maxShardReplicasDown=2")

	// TODO: Test for the overseer being the last node.

	// The overseer should be upgraded when given enough leeway
	maxshardReplicasUnavailable = intstr.FromString("50%")
	podsToUpgrade = getPodNames(pickPodsToUpgrade(solrCloud, lastPod, testDownClusterStatus, overseerLeader, 6, 2))
	assert.ElementsMatch(t, []string{"pod-0"}, podsToUpgrade, "Incorrect set of next pods to upgrade.")

	/*
		Test upgrades with a healthy cluster state.
	*/

	// Normal inputs
	maxshardReplicasUnavailable = intstr.FromInt(1)
	podsToUpgrade = getPodNames(pickPodsToUpgrade(solrCloud, halfPods, testHealthyClusterStatus, overseerLeader, 6, 6))
	assert.ElementsMatch(t, []string{"pod-5"}, podsToUpgrade, "Incorrect set of next pods to upgrade. Do to replica placement, only the node with the least leaders can be upgraded.")

	// Test the maxShardReplicasDownSpec
	maxshardReplicasUnavailable = intstr.FromInt(2)
	podsToUpgrade = getPodNames(pickPodsToUpgrade(solrCloud, halfPods, testHealthyClusterStatus, overseerLeader, 6, 6))
	assert.ElementsMatch(t, []string{"pod-1", "pod-5"}, podsToUpgrade, "Incorrect set of next pods to upgrade. More nodes should be upgraded when maxShardReplicasDown=2")

	// TODO: Test for the overseer being the last node.

	// The overseer should be upgraded when given enough leeway
	maxshardReplicasUnavailable = intstr.FromString("50%")
	podsToUpgrade = getPodNames(pickPodsToUpgrade(solrCloud, lastPod, testDownClusterStatus, overseerLeader, 6, 2))
	assert.ElementsMatch(t, []string{"pod-0"}, podsToUpgrade, "Incorrect set of next pods to upgrade.")

	/*
		Test the overseer node being taken down.
	*/

	// The overseer should be not be upgraded if the clusterstate is not healthy enough
	maxshardReplicasUnavailable = intstr.FromInt(1)
	podsToUpgrade = getPodNames(pickPodsToUpgrade(solrCloud, lastPod, testRecoveringClusterStatus, overseerLeader, 6, 3))
	assert.ElementsMatch(t, []string{}, podsToUpgrade, "Incorrect set of next pods to upgrade. The overseer should be not be upgraded if the clusterstate is not healthy enough.")

	// The overseer should be not be upgraded if the clusterstate is not healthy enough
	podsToUpgrade = getPodNames(pickPodsToUpgrade(solrCloud, lastPod, testRecoveringClusterStatus, overseerLeader, 6, 6))
	assert.ElementsMatch(t, []string{}, podsToUpgrade, "Incorrect set of next pods to upgrade. The overseer should be not be upgraded if there are other non-live nodes.")

	// The overseer should be upgraded when given enough leeway
	maxshardReplicasUnavailable = intstr.FromInt(2)
	podsToUpgrade = getPodNames(pickPodsToUpgrade(solrCloud, lastPod, testDownClusterStatus, overseerLeader, 6, 6))
	assert.ElementsMatch(t, []string{"pod-0"}, podsToUpgrade, "Incorrect set of next pods to upgrade. The overseer should be upgraded when given enough leeway.")

	// The overseer should be upgraded when everything is healthy and it is the last node
	maxshardReplicasUnavailable = intstr.FromInt(1)
	podsToUpgrade = getPodNames(pickPodsToUpgrade(solrCloud, lastPod, testHealthyClusterStatus, overseerLeader, 6, 6))
	assert.ElementsMatch(t, []string{"pod-0"}, podsToUpgrade, "Incorrect set of next pods to upgrade. The overseer should be upgraded when everything is healthy and it is the last node")
}

func TestPodUpgradeOrdering(t *testing.T) {
	solrCloud := &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
		Spec: solr.SolrCloudSpec{
			SolrAddressability: solr.SolrAddressabilityOptions{
				PodPort: 2000,
			},
		},
	}

	pods := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-0"}, Spec: corev1.PodSpec{}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-1"}, Spec: corev1.PodSpec{}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-2"}, Spec: corev1.PodSpec{}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-3"}, Spec: corev1.PodSpec{}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-4"}, Spec: corev1.PodSpec{}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-5"}, Spec: corev1.PodSpec{}},
	}

	nodeMap := map[string]SolrNodeContents{
		// This node should be last as it is the overseer
		SolrNodeName(solrCloud, pods[0]): {
			nodeName:       SolrNodeName(solrCloud, pods[0]),
			leaders:        4,
			overseerLeader: true,
			live:           true,
		},
		SolrNodeName(solrCloud, pods[1]): {
			nodeName:       SolrNodeName(solrCloud, pods[1]),
			leaders:        8,
			overseerLeader: false,
			live:           false,
		},
		SolrNodeName(solrCloud, pods[2]): {
			nodeName:       SolrNodeName(solrCloud, pods[2]),
			leaders:        0,
			overseerLeader: false,
			live:           true,
		},
		SolrNodeName(solrCloud, pods[3]): {
			nodeName:       SolrNodeName(solrCloud, pods[3]),
			leaders:        3,
			overseerLeader: false,
			live:           true,
		},
		// This node should come second to last as it is not the overseer, but it has the most leaders.
		SolrNodeName(solrCloud, pods[4]): {
			nodeName:       SolrNodeName(solrCloud, pods[4]),
			leaders:        10,
			overseerLeader: false,
			live:           true,
		},
		// This node should come after pod 3 since they are identically ordered, but the name pod-3 comes before pod-5.
		SolrNodeName(solrCloud, pods[5]): {
			nodeName:       SolrNodeName(solrCloud, pods[5]),
			leaders:        3,
			overseerLeader: false,
			live:           true,
		},
	}

	expectedOrdering := []string{"pod-2", "pod-5", "pod-3", "pod-1", "pod-4", "pod-0"}

	sortNodePodsBySafety(pods, nodeMap, solrCloud)
	foundOrdering := make([]string, len(pods))
	for i, pod := range pods {
		foundOrdering[i] = pod.Name
	}
	assert.EqualValues(t, expectedOrdering, foundOrdering, "Ordering of pods not correct.")
}

func TestFindSolrNodeContents(t *testing.T) {
	overseerLeader := "pod-0.foo-solrcloud-headless.default:2000_solr"

	nodeContents, totalShardReplicas, shardReplicasNotActive := findSolrNodeContents(testRecoveringClusterStatus, overseerLeader)

	expectedNodeContents := map[string]SolrNodeContents{
		"pod-0.foo-solrcloud-headless.default:2000_solr": {
			nodeName: "pod-0.foo-solrcloud-headless.default:2000_solr",
			leaders:  0,
			totalReplicasPerShard: map[string]int{
				"col1|shard1": 1,
				"col2|shard2": 1,
			},
			activeReplicasPerShard: map[string]int{
				"col1|shard1": 1,
			},
			downReplicasPerShard: map[string]int{
				"col2|shard2": 1,
			},
			overseerLeader: true,
			live:           true,
		},
		"pod-1.foo-solrcloud-headless.default:2000_solr": {
			nodeName: "pod-1.foo-solrcloud-headless.default:2000_solr",
			leaders:  1,
			totalReplicasPerShard: map[string]int{
				"col1|shard2": 1,
				"col2|shard1": 1,
			},
			activeReplicasPerShard: map[string]int{
				"col1|shard2": 1,
				"col2|shard1": 1,
			},
			downReplicasPerShard: map[string]int{},
			overseerLeader:       false,
			live:                 true,
		},
		"pod-2.foo-solrcloud-headless.default:2000_solr": {
			nodeName: "pod-2.foo-solrcloud-headless.default:2000_solr",
			leaders:  0,
			totalReplicasPerShard: map[string]int{
				"col1|shard1": 1,
				"col1|shard2": 1,
			},
			activeReplicasPerShard: map[string]int{
				"col1|shard2": 1,
			},
			downReplicasPerShard: map[string]int{},
			overseerLeader:       false,
			live:                 true,
		},
		"pod-3.foo-solrcloud-headless.default:2000_solr": {
			nodeName: "pod-3.foo-solrcloud-headless.default:2000_solr",
			leaders:  2,
			totalReplicasPerShard: map[string]int{
				"col1|shard1": 1,
				"col2|shard1": 1,
				"col2|shard2": 1,
			},
			activeReplicasPerShard: map[string]int{
				"col1|shard1": 1,
			},
			downReplicasPerShard: map[string]int{
				"col2|shard1": 1,
				"col2|shard2": 1,
			},
			overseerLeader: false,
			live:           true,
		},
		"pod-4.foo-solrcloud-headless.default:2000_solr": {
			nodeName: "pod-4.foo-solrcloud-headless.default:2000_solr",
			leaders:  0,
			totalReplicasPerShard: map[string]int{
				"col2|shard1": 1,
			},
			activeReplicasPerShard: map[string]int{},
			downReplicasPerShard:   map[string]int{},
			overseerLeader:         false,
			live:                   false,
		},
		"pod-5.foo-solrcloud-headless.default:2000_solr": {
			nodeName: "pod-5.foo-solrcloud-headless.default:2000_solr",
			leaders:  1,
			totalReplicasPerShard: map[string]int{
				"col1|shard2": 1,
				"col2|shard2": 2,
			},
			activeReplicasPerShard: map[string]int{
				"col2|shard2": 2,
			},
			downReplicasPerShard: map[string]int{},
			overseerLeader:       false,
			live:                 true,
		},
	}
	assert.Equal(t, 6, len(nodeContents), "Number of Solr nodes with content information is incorrect.")
	for node, expectedContents := range expectedNodeContents {
		foundNodeContents, found := nodeContents[node]
		assert.Truef(t, found, "No nodeContents found for node %s", node)
		assert.EqualValuesf(t, expectedContents, foundNodeContents, "NodeContents information from clusterstate is incorrect for node %s", node)
	}

	expectedTotalShardReplicas := map[string]int{
		"col1|shard1": 3,
		"col1|shard2": 3,
		"col2|shard1": 3,
		"col2|shard2": 4,
	}
	assert.EqualValues(t, expectedTotalShardReplicas, totalShardReplicas, "Shards replica count is incorrect.")

	expectedShardReplicasNotActive := map[string]int{
		"col1|shard1": 1,
		"col1|shard2": 1,
		"col2|shard1": 2,
		"col2|shard2": 2,
	}
	assert.EqualValues(t, expectedShardReplicasNotActive, shardReplicasNotActive, "Shards with replicas not active information is incorrect.")
}

func TestCalculateMaxPodsToUpgrade(t *testing.T) {
	maxPodsUnavailable := intstr.FromInt(2)

	solrCloud := &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
		Spec: solr.SolrCloudSpec{
			SolrAddressability: solr.SolrAddressabilityOptions{
				PodPort: 2000,
			},
			UpdateStrategy: solr.SolrUpdateStrategy{
				Method: solr.ManagedUpdate,
				ManagedUpdateOptions: solr.ManagedUpdateOptions{
					MaxPodsUnavailable: &maxPodsUnavailable,
				},
			},
		},
	}

	foundMaxPodsUnavailable, foundMaxPodsToUpdate := calculateMaxPodsToUpdate(solrCloud, 10, 2)
	assert.Equal(t, 2, foundMaxPodsUnavailable, "Incorrect value of maxPodsUnavailable given fromInt(2)")
	assert.Equal(t, 0, foundMaxPodsToUpdate, "Incorrect value of foundMaxPodsToUpdate")

	foundMaxPodsUnavailable, foundMaxPodsToUpdate = calculateMaxPodsToUpdate(solrCloud, 10, 3)
	assert.Equal(t, 2, foundMaxPodsUnavailable, "Incorrect value of maxPodsUnavailable given fromInt(2)")
	assert.Equal(t, -1, foundMaxPodsToUpdate, "Incorrect value of foundMaxPodsToUpdate")

	maxPodsUnavailable = intstr.FromString("45%")
	foundMaxPodsUnavailable, foundMaxPodsToUpdate = calculateMaxPodsToUpdate(solrCloud, 10, 2)
	assert.Equal(t, 4, foundMaxPodsUnavailable, "Incorrect value of maxPodsUnavailable given fromInt(2)")
	assert.Equal(t, 2, foundMaxPodsToUpdate, "Incorrect value of foundMaxPodsToUpdate")

	maxPodsUnavailable = intstr.FromString("70%")
	foundMaxPodsUnavailable, foundMaxPodsToUpdate = calculateMaxPodsToUpdate(solrCloud, 10, 5)
	assert.Equal(t, 7, foundMaxPodsUnavailable, "Incorrect value of maxPodsUnavailable given fromInt(2)")
	assert.Equal(t, 2, foundMaxPodsToUpdate, "Incorrect value of foundMaxPodsToUpdate")
}

func TestSolrNodeName(t *testing.T) {
	solrCloud := &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
		Spec: solr.SolrCloudSpec{
			SolrAddressability: solr.SolrAddressabilityOptions{
				PodPort:           2000,
				CommonServicePort: 80,
			},
		},
	}

	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-0"},
		Spec:       corev1.PodSpec{},
	}

	assert.Equal(t, "pod-0.foo-solrcloud-headless.default:2000_solr", SolrNodeName(solrCloud, pod), "Incorrect generation of Solr nodeName")

	solrCloud.Spec.SolrAddressability.PodPort = 3000
	assert.Equal(t, "pod-0.foo-solrcloud-headless.default:3000_solr", SolrNodeName(solrCloud, pod), "Incorrect generation of Solr nodeName")
}

var (
	testRecoveringClusterStatus = solr_api.SolrClusterStatus{
		LiveNodes: []string{
			"pod-0.foo-solrcloud-headless.default:2000_solr",
			"pod-1.foo-solrcloud-headless.default:2000_solr",
			"pod-2.foo-solrcloud-headless.default:2000_solr",
			"pod-3.foo-solrcloud-headless.default:2000_solr",
			"pod-5.foo-solrcloud-headless.default:2000_solr",
		},
		Collections: map[string]solr_api.SolrCollectionStatus{
			"col1": {
				Shards: map[string]solr_api.SolrShardStatus{
					"shard1": {
						Replicas: map[string]solr_api.SolrReplicaStatus{
							"rep-1-1-1": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "pod-0.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "pod-0.foo-solrcloud-headless.default:2000/solr/rep-1-1-1",
								Leader:   false,
								Type:     solr_api.PULL,
							},
							"rep-1-1-2": {
								State:    solr_api.ReplicaRecovering,
								Core:     "core1",
								NodeName: "pod-2.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "pod-2.foo-solrcloud-headless.default:2000/solr/rep-1-1-2",
								Leader:   false,
								Type:     solr_api.TLOG,
							},
							"rep-1-1-3": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "pod-3.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "pod-3.foo-solrcloud-headless.default:2000/solr/rep-1-1-3",
								Leader:   true,
								Type:     solr_api.TLOG,
							},
						},
						State: solr_api.ShardActive,
					},
					"shard2": {
						Replicas: map[string]solr_api.SolrReplicaStatus{
							"rep-1-2-1": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "pod-2.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "pod-2.foo-solrcloud-headless.default:2000/solr/rep-1-2-1",
								Leader:   false,
								Type:     solr_api.NRT,
							},
							"rep-1-2-2": {
								State:    solr_api.ReplicaRecovering,
								Core:     "core1",
								NodeName: "pod-5.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "pod-5.foo-solrcloud-headless.default:2000/solr/rep-1-2-2",
								Leader:   false,
								Type:     solr_api.NRT,
							},
							"rep-1-2-3": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "pod-1.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "pod-1.foo-solrcloud-headless.default:2000/solr/rep-1-2-3",
								Leader:   true,
								Type:     solr_api.NRT,
							},
						},
						State: solr_api.ShardActive,
					},
				},
				ConfigName: "test",
			},
			"col2": {
				Shards: map[string]solr_api.SolrShardStatus{
					"shard1": {
						Replicas: map[string]solr_api.SolrReplicaStatus{
							"rep-2-1-1": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "pod-4.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "pod-4.foo-solrcloud-headless.default:2000/solr/rep-2-1-1",
								Leader:   false,
								Type:     solr_api.PULL,
							},
							"rep-2-1-2": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "pod-1.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "pod-1.foo-solrcloud-headless.default:2000/solr/rep-2-1-2",
								Leader:   false,
								Type:     solr_api.TLOG,
							},
							"rep-2-1-3": {
								State:    solr_api.ReplicaDown,
								Core:     "core1",
								NodeName: "pod-3.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "pod-3.foo-solrcloud-headless.default:2000/solr/rep-2-1-3",
								Leader:   true,
								Type:     solr_api.TLOG,
							},
						},
						State: solr_api.ShardActive,
					},
					"shard2": {
						Replicas: map[string]solr_api.SolrReplicaStatus{
							"rep-2-2-1": {
								State:    solr_api.ReplicaDown,
								Core:     "core1",
								NodeName: "pod-3.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "pod-3.foo-solrcloud-headless.default:2000/solr/rep-2-2-1",
								Leader:   false,
								Type:     solr_api.NRT,
							},
							"rep-2-2-2": {
								State:    solr_api.ReplicaRecoveryFailed,
								Core:     "core1",
								NodeName: "pod-0.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "pod-0.foo-solrcloud-headless.default:2000/solr/rep-2-2-2",
								Leader:   false,
								Type:     solr_api.NRT,
							},
							"rep-2-2-3": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "pod-5.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "pod-5.foo-solrcloud-headless.default:2000/solr/rep-2-2-3",
								Leader:   true,
								Type:     solr_api.NRT,
							},
							"rep-2-2-4": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "pod-5.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "pod-5.foo-solrcloud-headless.default:2000/solr/rep-2-2-4",
								Leader:   false,
								Type:     solr_api.NRT,
							},
						},
						State: solr_api.ShardActive,
					},
				},
				ConfigName: "test",
			},
		},
	}

	testDownClusterStatus = solr_api.SolrClusterStatus{
		LiveNodes: []string{
			"pod-0.foo-solrcloud-headless.default:2000_solr",
			"pod-1.foo-solrcloud-headless.default:2000_solr",
			"pod-2.foo-solrcloud-headless.default:2000_solr",
			"pod-3.foo-solrcloud-headless.default:2000_solr",
			"pod-4.foo-solrcloud-headless.default:2000_solr",
			"pod-5.foo-solrcloud-headless.default:2000_solr",
		},
		Collections: map[string]solr_api.SolrCollectionStatus{
			"col1": {
				Shards: map[string]solr_api.SolrShardStatus{
					"shard1": {
						Replicas: map[string]solr_api.SolrReplicaStatus{
							"rep-1-1-1": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "pod-0.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "pod-0.foo-solrcloud-headless.default:2000/solr/rep-1-1-1",
								Leader:   false,
								Type:     solr_api.PULL,
							},
							"rep-1-1-2": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "pod-2.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "pod-2.foo-solrcloud-headless.default:2000/solr/rep-1-1-2",
								Leader:   false,
								Type:     solr_api.TLOG,
							},
							"rep-1-1-3": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "pod-3.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "pod-3.foo-solrcloud-headless.default:2000/solr/rep-1-1-3",
								Leader:   true,
								Type:     solr_api.TLOG,
							},
						},
						State: solr_api.ShardActive,
					},
					"shard2": {
						Replicas: map[string]solr_api.SolrReplicaStatus{
							"rep-1-2-1": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "pod-2.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "pod-2.foo-solrcloud-headless.default:2000/solr/rep-1-2-1",
								Leader:   false,
								Type:     solr_api.NRT,
							},
							"rep-1-2-2": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "pod-5.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "pod-5.foo-solrcloud-headless.default:2000/solr/rep-1-2-2",
								Leader:   false,
								Type:     solr_api.NRT,
							},
							"rep-1-2-3": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "pod-1.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "pod-1.foo-solrcloud-headless.default:2000/solr/rep-1-2-3",
								Leader:   true,
								Type:     solr_api.NRT,
							},
						},
						State: solr_api.ShardActive,
					},
				},
				ConfigName: "test",
			},
			"col2": {
				Shards: map[string]solr_api.SolrShardStatus{
					"shard1": {
						Replicas: map[string]solr_api.SolrReplicaStatus{
							"rep-2-1-1": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "pod-4.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "pod-4.foo-solrcloud-headless.default:2000/solr/rep-2-1-1",
								Leader:   false,
								Type:     solr_api.PULL,
							},
							"rep-2-1-2": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "pod-1.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "pod-1.foo-solrcloud-headless.default:2000/solr/rep-2-1-2",
								Leader:   false,
								Type:     solr_api.TLOG,
							},
							"rep-2-1-3": {
								State:    solr_api.ReplicaDown,
								Core:     "core1",
								NodeName: "pod-3.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "pod-3.foo-solrcloud-headless.default:2000/solr/rep-2-1-3",
								Leader:   true,
								Type:     solr_api.TLOG,
							},
						},
						State: solr_api.ShardActive,
					},
					"shard2": {
						Replicas: map[string]solr_api.SolrReplicaStatus{
							"rep-2-2-1": {
								State:    solr_api.ReplicaDown,
								Core:     "core1",
								NodeName: "pod-3.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "pod-3.foo-solrcloud-headless.default:2000/solr/rep-2-2-1",
								Leader:   false,
								Type:     solr_api.NRT,
							},
							"rep-2-2-2": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "pod-0.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "pod-0.foo-solrcloud-headless.default:2000/solr/rep-2-2-2",
								Leader:   false,
								Type:     solr_api.NRT,
							},
							"rep-2-2-3": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "pod-5.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "pod-5.foo-solrcloud-headless.default:2000/solr/rep-2-2-3",
								Leader:   true,
								Type:     solr_api.NRT,
							},
							"rep-2-2-4": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "pod-5.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "pod-5.foo-solrcloud-headless.default:2000/solr/rep-2-2-4",
								Leader:   false,
								Type:     solr_api.NRT,
							},
						},
						State: solr_api.ShardActive,
					},
				},
				ConfigName: "test",
			},
		},
	}

	testHealthyClusterStatus = solr_api.SolrClusterStatus{
		LiveNodes: []string{
			"pod-0.foo-solrcloud-headless.default:2000_solr",
			"pod-1.foo-solrcloud-headless.default:2000_solr",
			"pod-2.foo-solrcloud-headless.default:2000_solr",
			"pod-3.foo-solrcloud-headless.default:2000_solr",
			"pod-4.foo-solrcloud-headless.default:2000_solr",
			"pod-5.foo-solrcloud-headless.default:2000_solr",
		},
		Collections: map[string]solr_api.SolrCollectionStatus{
			"col1": {
				Shards: map[string]solr_api.SolrShardStatus{
					"shard1": {
						Replicas: map[string]solr_api.SolrReplicaStatus{
							"rep-1-1-1": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "pod-0.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "pod-0.foo-solrcloud-headless.default:2000/solr/rep-1-1-1",
								Leader:   false,
								Type:     solr_api.PULL,
							},
							"rep-1-1-2": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "pod-2.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "pod-2.foo-solrcloud-headless.default:2000/solr/rep-1-1-2",
								Leader:   false,
								Type:     solr_api.TLOG,
							},
							"rep-1-1-3": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "pod-3.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "pod-3.foo-solrcloud-headless.default:2000/solr/rep-1-1-3",
								Leader:   true,
								Type:     solr_api.TLOG,
							},
						},
						State: solr_api.ShardActive,
					},
					"shard2": {
						Replicas: map[string]solr_api.SolrReplicaStatus{
							"rep-1-2-1": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "pod-2.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "pod-2.foo-solrcloud-headless.default:2000/solr/rep-1-2-1",
								Leader:   false,
								Type:     solr_api.NRT,
							},
							"rep-1-2-2": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "pod-5.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "pod-5.foo-solrcloud-headless.default:2000/solr/rep-1-2-2",
								Leader:   false,
								Type:     solr_api.NRT,
							},
							"rep-1-2-3": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "pod-1.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "pod-1.foo-solrcloud-headless.default:2000/solr/rep-1-2-3",
								Leader:   true,
								Type:     solr_api.NRT,
							},
						},
						State: solr_api.ShardActive,
					},
				},
				ConfigName: "test",
			},
			"col2": {
				Shards: map[string]solr_api.SolrShardStatus{
					"shard1": {
						Replicas: map[string]solr_api.SolrReplicaStatus{
							"rep-2-1-1": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "pod-4.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "pod-4.foo-solrcloud-headless.default:2000/solr/rep-2-1-1",
								Leader:   false,
								Type:     solr_api.PULL,
							},
							"rep-2-1-2": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "pod-1.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "pod-1.foo-solrcloud-headless.default:2000/solr/rep-2-1-2",
								Leader:   false,
								Type:     solr_api.TLOG,
							},
							"rep-2-1-3": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "pod-3.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "pod-3.foo-solrcloud-headless.default:2000/solr/rep-2-1-3",
								Leader:   true,
								Type:     solr_api.TLOG,
							},
						},
						State: solr_api.ShardActive,
					},
					"shard2": {
						Replicas: map[string]solr_api.SolrReplicaStatus{
							"rep-2-2-1": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "pod-3.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "pod-3.foo-solrcloud-headless.default:2000/solr/rep-2-2-1",
								Leader:   false,
								Type:     solr_api.NRT,
							},
							"rep-2-2-2": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "pod-0.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "pod-0.foo-solrcloud-headless.default:2000/solr/rep-2-2-2",
								Leader:   false,
								Type:     solr_api.NRT,
							},
							"rep-2-2-3": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "pod-5.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "pod-5.foo-solrcloud-headless.default:2000/solr/rep-2-2-3",
								Leader:   true,
								Type:     solr_api.NRT,
							},
							"rep-2-2-4": {
								State:    solr_api.ReplicaActive,
								Core:     "core1",
								NodeName: "pod-5.foo-solrcloud-headless.default:2000_solr",
								BaseUrl:  "pod-5.foo-solrcloud-headless.default:2000/solr/rep-2-2-4",
								Leader:   false,
								Type:     solr_api.NRT,
							},
						},
						State: solr_api.ShardActive,
					},
				},
				ConfigName: "test",
			},
		},
	}
)

func getPodNames(pods []corev1.Pod) []string {
	names := make([]string, len(pods))
	for i, pod := range pods {
		names[i] = pod.Name
	}
	return names
}
