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
	"fmt"
	solr "github.com/bloomberg/solr-operator/api/v1beta1"
	"github.com/bloomberg/solr-operator/controllers/util/solr_api"
	corev1 "k8s.io/api/core/v1"
	"math"
	"net/url"
	"sort"
)

// DeterminePodsSafeToUpgrade takes a list of solr Pods and returns a list of pods that are safe to upgrade now.
// This function MUST be idempotent and return the same list of pods given the same kubernetes/solr state.
// TODO:
//  - The current down replicas are not taken into consideration
//  - If the replica in the shard is not active should it matter?
//  - Think about caching this for ~250 ms? Not a huge need to send these requests milliseconds apart.
//    - Might be too much complexity for very little gain.
//  - Probably want to give options for upgrade
//    - Controlled: Managed by the Solr Operator
//      - maxShardReplicasDown (defaults to 1)
//		- maxBatchNodeUpgrade (<=0 is unlimited, 0-1 is a percentage of total nodes, >1 is a hard number)
//    - Manual: Managed by the users deleting their own pods.
func DeterminePodsSafeToUpgrade(cloud *solr.SolrCloud, outOfDatePods []corev1.Pod, totalPods int, readyPods int) (podsToUpgrade []corev1.Pod, err error) {
	queryParams := url.Values{}
	queryParams.Add("action", "CLUSTERSTATUS")
	clusterResp := &solr_api.SolrClusterStatusResponse{}
	overseerResp := &solr_api.SolrOverseerStatusResponse{}

	// TODO: Make these configurable?
	maxShardReplicasDown := 1.0
	maxBatchNodeUpgradeSpec := .5

	if readyPods > 0 {
		err = solr_api.CallCollectionsApi(cloud.Name, cloud.Namespace, queryParams, clusterResp)
		if err == nil {
			if hasError, apiErr := solr_api.CheckForCollectionsApiError("CLUSTERSTATUS", clusterResp.ResponseHeader); hasError {
				err = apiErr
			} else {
				queryParams.Set("action", "OVERSEERSTATUS")
				err = solr_api.CallCollectionsApi(cloud.Name, cloud.Namespace, queryParams, overseerResp)
				if hasError, apiErr := solr_api.CheckForCollectionsApiError("OVERSEERSTATUS", clusterResp.ResponseHeader); hasError {
					err = apiErr
				}
			}
		}
	}
	if err != nil {
		log.Error(err, "Error retrieving cluster status", "namespace", cloud.Namespace, "cloud", cloud.Name)
	} else {
		podsToUpgrade = pickPodsToUpgrade(cloud, outOfDatePods, clusterResp.ClusterStatus, overseerResp.Leader, totalPods, maxShardReplicasDown, maxBatchNodeUpgradeSpec)
	}
	return podsToUpgrade, err
}

func pickPodsToUpgrade(cloud *solr.SolrCloud, outOfDatePods []corev1.Pod, clusterStatus solr_api.SolrClusterStatus,
	overseer string, totalPods int, maxShardReplicasDownSpec float64, maxBatchNodeUpgradeSpec float64) (podsToUpgrade []corev1.Pod) {

	nodeContents, totalShardReplicas, shardReplicasNotActive := findSolrNodeContents(clusterStatus, overseer)
	sortNodePodsBySafety(outOfDatePods, nodeContents, cloud)

	// If the maxBatchNodeUpgradeSpec is passed as a decimal between 0 and 1, then calculate as a percentage of the number of nodes.
	maxBatchNodeUpgrade := int(maxBatchNodeUpgradeSpec)
	if maxBatchNodeUpgradeSpec > 0 && maxBatchNodeUpgradeSpec < 1 {
		maxBatchNodeUpgrade = int(math.Ceil(maxBatchNodeUpgradeSpec * float64(totalPods)))
	}

	for _, pod := range outOfDatePods {
		isSafe := true
		nodeName := SolrNodeName(cloud, pod)
		nodeContent, isInClusterState := nodeContents[nodeName]
		fmt.Printf("%s: %t %+v\n", pod.Name, isInClusterState, nodeContent)
		// The overseerLeader can only be upgraded by itself
		if !isInClusterState {
			// All pods not in the cluster state are safe to upgrade
			isSafe = true
			fmt.Printf("%s: Killed. Reason - not in cluster state\n", pod.Name)
		} else if nodeContent.overseerLeader {
			// The overseerLeader can only be upgraded by itself
			// We want to update it when it's the last out of date pods and all nodes are "live"
			if len(outOfDatePods) == 1 && len(clusterStatus.LiveNodes) == totalPods {
				isSafe = true
				fmt.Printf("%s: Killed. Reason - overseerLeader & last node\n", pod.Name)
			} else {
				isSafe = false
				fmt.Printf("%s: Saved. Reason - overseerLeader\n", pod.Name)
			}
		} else {
			for shard, additionalReplicaCount := range nodeContent.totalReplicasPerShard {
				// If all of the replicas for a shard on the node are down, then this is safe to kill.
				// Currently this logic lets replicas in recovery continue recovery rather than killing them.
				// TODO: Make sure this logic makes sense
				if additionalReplicaCount == nodeContent.downReplicasPerShard[shard] || !nodeContent.live {
					continue
				}

				notActiveReplicaCount, _ := shardReplicasNotActive[shard]

				// We have to allow killing of Pods that have multiple replicas of a shard
				// Therefore only check the additional Replica count if some replicas of that shard are already being upgraded
				// Also we only want to check the addition of the active replicas, as the non-active replicas are already included in the check.
				// If the maxBatchNodeUpgradeSpec is passed as a decimal between 0 and 1, then calculate as a percentage of the number of nodes.
				maxShardReplicasDown := int(maxShardReplicasDownSpec)
				if maxShardReplicasDownSpec > 0 && maxShardReplicasDownSpec < 1 {
					maxShardReplicasDown = int(math.Floor(maxShardReplicasDownSpec * float64(totalShardReplicas[shard])))
				}
				if notActiveReplicaCount > 0 && notActiveReplicaCount+nodeContent.activeReplicasPerShard[shard] > maxShardReplicasDown {
					fmt.Printf("%s: Saved. Reason - shard %s already has %d replicas not active, taking down %d more would put it over the maximum allowed down: %d\n", pod.Name, shard, notActiveReplicaCount, nodeContent.activeReplicasPerShard[shard], maxShardReplicasDown)
					isSafe = false
					break
				}
			}
			if isSafe {
				fmt.Printf("%s: Killed. Reason - does not conflict with totalReplicasPerShard\n", pod.Name)
			}
		}
		if isSafe {
			for shard, additionalReplicaCount := range nodeContent.activeReplicasPerShard {
				shardReplicasNotActive[shard] += additionalReplicaCount
			}
			podsToUpgrade = append(podsToUpgrade, pod)

			// Stop after the maxBatchNodeUpgrade count, if one is provided.
			if maxBatchNodeUpgrade >= 1 && len(podsToUpgrade) >= maxBatchNodeUpgrade {
				break
			}
		}
	}
	return podsToUpgrade
}

func sortNodePodsBySafety(outOfDatePods []corev1.Pod, nodeMap map[string]SolrNodeContents, solrCloud *solr.SolrCloud) {
	sort.SliceStable(outOfDatePods, func(i, j int) bool {
		nodeI, hasNodeI := nodeMap[SolrNodeName(solrCloud, outOfDatePods[i])]
		if !hasNodeI {
			return true
		} else if nodeI.overseerLeader {
			return false
		}
		nodeJ, hasNodeJ := nodeMap[SolrNodeName(solrCloud, outOfDatePods[j])]
		if !hasNodeJ {
			return false
		} else if nodeJ.overseerLeader {
			return true
		}

		if nodeI.leaders == nodeJ.leaders {
			if nodeI.nodeName < nodeJ.nodeName {
				return false // sort largest ordinal first
			}
			return true
		} else {
			return nodeI.leaders < nodeJ.leaders
		}
	})
}

/*
FindSolrNodeContents will take a cluster and overseerLeader response from the SolrCloud Collections API, and aggregate the information.
This aggregated info is returned as:
	- A map from Solr nodeName to SolrNodeContents, with the information from the clusterState and overseerLeader
    - A map from unique shard name (collection+shard) to the count of replicas that are not active for that shard.
*/
func findSolrNodeContents(cluster solr_api.SolrClusterStatus, overseerLeader string) (nodeContents map[string]SolrNodeContents, totalShardReplicas map[string]int, shardReplicasNotActive map[string]int) {
	nodeContents = map[string]SolrNodeContents{}
	totalShardReplicas = map[string]int{}
	shardReplicasNotActive = map[string]int{}
	// Update the info for each "live" node.
	for _, nodeName := range cluster.LiveNodes {
		contents, hasValue := nodeContents[nodeName]
		if !hasValue {
			contents = SolrNodeContents{
				nodeName:               nodeName,
				leaders:                0,
				totalReplicasPerShard:  map[string]int{},
				activeReplicasPerShard: map[string]int{},
				downReplicasPerShard:   map[string]int{},
				overseerLeader:         false,
				live:                   true,
			}
		} else {
			contents.live = true
		}
		nodeContents[nodeName] = contents
	}
	// Go through the state of each collection getting the count of replicas for each collection/shard living on each node
	for collectionName, collection := range cluster.Collections {
		for shardName, shard := range collection.Shards {
			uniqueShard := collectionName + "|" + shardName
			totalShardReplicas[uniqueShard] = len(shard.Replicas)
			for _, replica := range shard.Replicas {
				contents, hasValue := nodeContents[replica.NodeName]
				if !hasValue {
					contents = SolrNodeContents{
						nodeName:               replica.NodeName,
						leaders:                0,
						totalReplicasPerShard:  map[string]int{},
						activeReplicasPerShard: map[string]int{},
						downReplicasPerShard:   map[string]int{},
						overseerLeader:         false,
						live:                   false,
					}
				}
				if replica.Leader {
					contents.leaders += 1
				}
				contents.totalReplicasPerShard[uniqueShard] += 1

				// A shard can be considered "not active" if it's state is not "active" or the node it lives in is not "live".
				if replica.State != solr_api.ReplicaActive || !contents.live {
					shardReplicasNotActive[uniqueShard] += 1
				} else {
					contents.activeReplicasPerShard[uniqueShard] += 1
				}
				// Keep track of how many of the replicas of this shard are in a down state (down or recovery_failed)
				if replica.State == solr_api.ReplicaDown || replica.State == solr_api.ReplicaRecoveryFailed {
					contents.downReplicasPerShard[uniqueShard] += 1
				}

				nodeContents[replica.NodeName] = contents
			}
		}
	}
	// Update the info for the overseerLeader leader.
	if overseerLeader != "" {
		contents, hasValue := nodeContents[overseerLeader]
		if !hasValue {
			contents = SolrNodeContents{
				nodeName:               overseerLeader,
				leaders:                0,
				totalReplicasPerShard:  map[string]int{},
				activeReplicasPerShard: map[string]int{},
				downReplicasPerShard:   map[string]int{},
				overseerLeader:         true,
				live:                   false,
			}
		} else {
			contents.overseerLeader = true
		}
		nodeContents[overseerLeader] = contents
	}
	return nodeContents, totalShardReplicas, shardReplicasNotActive
}

type SolrNodeContents struct {
	// The name of the Solr Node (or pod)
	nodeName string

	// The number of leader replicas that reside in this Solr Node
	leaders int

	// The number of total replicas in this Solr Node, grouped by each unique shard (collection+shard)
	totalReplicasPerShard map[string]int

	// The number of active replicas in this Solr Node, grouped by each unique shard (collection+shard)
	activeReplicasPerShard map[string]int

	// The number of down (or recovery_failed) replicas in this Solr Node, grouped by each unique shard (collection+shard)
	downReplicasPerShard map[string]int

	// Whether this SolrNode is the overseer leader in the SolrCloud
	overseerLeader bool

	// Whether this SolrNode is registered as a live node in the SolrCloud (Alive and connected to ZK)
	live bool
}

// SolrNodeName takes a cloud and a pod and returns the Solr nodeName for that pod
func SolrNodeName(solrCloud *solr.SolrCloud, pod corev1.Pod) string {
	return fmt.Sprintf("%s:%d_solr", solrCloud.AdvertisedNodeHost(pod.Name), solrCloud.NodePort())
}
