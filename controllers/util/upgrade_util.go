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
	"k8s.io/apimachinery/pkg/util/intstr"
	"net/url"
	"sort"
)

const (
	DefaultMaxPodsUnavailable          = "25%"
	DefaultMaxShardReplicasUnavailable = 1
)

// DeterminePodsSafeToUpgrade takes a list of solr Pods and returns a list of pods that are safe to upgrade now.
// This function MUST be idempotent and return the same list of pods given the same kubernetes/solr state.
// TODO:
//  - The current down replicas are not taken into consideration
//  - If the replica in the shard is not active should it matter?
//  - Think about caching this for ~250 ms? Not a huge need to send these requests milliseconds apart.
//    - Might be too much complexity for very little gain.
//  - Change Prints to logs
func DeterminePodsSafeToUpgrade(cloud *solr.SolrCloud, outOfDatePods []corev1.Pod, totalPods int, readyPods int, availableUpdatedPodCount int) (podsToUpgrade []corev1.Pod, err error) {
	// Before fetching the cluster state, be sure that there is room to update at least 1 pod
	maxPodsUnavailable, unavailableUpdatedPodCount, maxPodsToUpdate := calculateMaxPodsToUpdate(cloud, totalPods, len(outOfDatePods), availableUpdatedPodCount)
	if maxPodsToUpdate <= 0 {
		log.Info("Pod update selection canceled. The number of updated pods unavailable equals or exceeds the calculated maxPodsUnavailable.", "unavailableUpdatedPods", unavailableUpdatedPodCount, "maxPodsUnavailable", maxPodsUnavailable)
	} else {
		queryParams := url.Values{}
		queryParams.Add("action", "CLUSTERSTATUS")
		clusterResp := &solr_api.SolrClusterStatusResponse{}
		overseerResp := &solr_api.SolrOverseerStatusResponse{}

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
			log.Error(err, "Error retrieving cluster status, aborting pod update selection", "namespace", cloud.Namespace, "cloud", cloud.Name)
		} else {
			log.Info("Pod update selection started.", "outOfDatePods", len(outOfDatePods), "maxPodsUnavailable", maxPodsUnavailable, "unavailableUpdatedPods", unavailableUpdatedPodCount, "maxPodsToUpdate", maxPodsToUpdate)
			podsToUpgrade = pickPodsToUpgrade(cloud, outOfDatePods, clusterResp.ClusterStatus, overseerResp.Leader, totalPods, maxPodsToUpdate)
		}
	}
	return podsToUpgrade, err
}

// calculateMaxPodsToUpdate determines the maximum number of additional pods that can be updated.
func calculateMaxPodsToUpdate(cloud *solr.SolrCloud, totalPods int, outOfDatePodCount int, availableUpdatedPodCount int) (maxPodsUnavailable int, unavailableUpdatedPodCount int, maxPodsToUpdate int) {
	// In order to calculate the number of updated pods that are unavailable take all pods, take the total pods and subtract those that are available and updated, and those that are not updated.
	unavailableUpdatedPodCount = totalPods - availableUpdatedPodCount - outOfDatePodCount
	// If the maxBatchNodeUpgradeSpec is passed as a decimal between 0 and 1, then calculate as a percentage of the number of nodes.
	maxPodsUnavailable, _ = ResolveMaxPodsUnavailable(cloud.Spec.UpdateStrategy.ManagedUpdateOptions.MaxPodsUnavailable, totalPods)
	return maxPodsUnavailable, unavailableUpdatedPodCount, maxPodsUnavailable - unavailableUpdatedPodCount
}

func pickPodsToUpgrade(cloud *solr.SolrCloud, outOfDatePods []corev1.Pod, clusterStatus solr_api.SolrClusterStatus,
	overseer string, totalPods int, maxPodsToUpdate int) (podsToUpdate []corev1.Pod) {

	nodeContents, totalShardReplicas, shardReplicasNotActive := findSolrNodeContents(clusterStatus, overseer)
	sortNodePodsBySafety(outOfDatePods, nodeContents, cloud)

	updateOptions := cloud.Spec.UpdateStrategy.ManagedUpdateOptions
	var maxShardReplicasUnavailableCache map[string]int
	// In case the user wants all shardReplicas to be unavailable at the same time, populate the cache with the total number of replicas per shard.
	if updateOptions.MaxShardReplicasUnavailable.Type == intstr.Int && updateOptions.MaxShardReplicasUnavailable.IntVal <= int32(0) {
		maxShardReplicasUnavailableCache = totalShardReplicas
	} else {
		maxShardReplicasUnavailableCache = make(map[string]int, len(totalShardReplicas))
	}

	// TODO: Check to see if any of the podsToUpgrade are down. If so go ahead and update them? (Need to think more on this)
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
				// If the maxBatchNodeUpgradeSpec is passed as a decimal between 0 and 1, then calculate as a percentage of the number of nodes
				maxShardReplicasDown, _ := ResolveMaxShardReplicasUnavailable(updateOptions.MaxShardReplicasUnavailable, shard, totalShardReplicas, maxShardReplicasUnavailableCache)

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
			podsToUpdate = append(podsToUpdate, pod)

			// Stop after the maxBatchNodeUpdate count, if one is provided.
			if maxPodsToUpdate >= 1 && len(podsToUpdate) >= maxPodsToUpdate {
				log.Info("Pod update selection complete. Maximum number of pods able to be updated reached.", "cloud", cloud.Name, "namespace", cloud.Namespace, "maxPodsToUpdate", maxPodsToUpdate)
				break
			}
		}
	}
	return podsToUpdate
}

func sortNodePodsBySafety(outOfDatePods []corev1.Pod, nodeMap map[string]SolrNodeContents, solrCloud *solr.SolrCloud) {
	sort.SliceStable(outOfDatePods, func(i, j int) bool {
		// First sort by if the node is in the ClusterState
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

		// If both nodes are in the ClusterState and not overseerLeader, then prioritize the one with less leaders.
		if nodeI.leaders != nodeJ.leaders {
			return nodeI.leaders < nodeJ.leaders
		}

		// If the nodes have the same number of leaders, then prioritize by the number of replicas.
		if nodeI.replicas != nodeJ.replicas {
			return nodeI.replicas < nodeJ.replicas
		}

		// If the nodes have the same number of replicas, then prioritize if one node is not live.
		if nodeI.live != nodeJ.live {
			return !nodeI.live
		}

		// Lastly break any ties by a comparison of the name
		return nodeI.nodeName > nodeJ.nodeName
	})
}

// ResolveMaxPodsUnavailable resolves the maximum number of pods that are allowed to be unavailable, when choosing pods to update.
func ResolveMaxPodsUnavailable(maxPodsUnavailable *intstr.IntOrString, desiredPods int) (int, error) {
	if maxPodsUnavailable.Type == intstr.Int && maxPodsUnavailable.IntVal <= int32(0) {
		return desiredPods, nil
	}
	podsUnavailable, err := intstr.GetValueFromIntOrPercent(intstr.ValueOrDefault(maxPodsUnavailable, intstr.FromString(DefaultMaxPodsUnavailable)), desiredPods, false)
	if err != nil {
		return 1, err
	}

	if podsUnavailable == 0 {
		// podsUnavailable can never be 0, otherwise pods would never be able to be upgraded.
		podsUnavailable = 1
	}

	return podsUnavailable, nil
}

// ResolveMaxShardReplicasUnavailable resolves the maximum number of replicas that are allowed to be unavailable for a given shard, when choosing pods to update.
func ResolveMaxShardReplicasUnavailable(maxShardReplicasUnavailable *intstr.IntOrString, shard string, totalShardReplicas map[string]int, cache map[string]int) (int, error) {
	maxUnavailable, isCached := cache[shard]
	var err error
	if !isCached {
		maxUnavailable, err = intstr.GetValueFromIntOrPercent(intstr.ValueOrDefault(maxShardReplicasUnavailable, intstr.FromInt(DefaultMaxShardReplicasUnavailable)), totalShardReplicas[shard], false)
		if err != nil {
			maxUnavailable = 1
		}
	} else {
		err = nil
	}

	if maxUnavailable == 0 {
		// podsUnavailable can never be 0, otherwise pods would never be able to be upgraded.
		maxUnavailable = 1
	}

	return maxUnavailable, nil
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
				replicas:               0,
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
						replicas:               0,
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
				contents.replicas += 1
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

	// The number of replicas that reside in this Solr Node
	replicas int

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
