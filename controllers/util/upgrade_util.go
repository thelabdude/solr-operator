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
func DeterminePodsSafeToUpgrade(cloud *solr.SolrCloud, outOfDatePods []corev1.Pod, totalNodes int32, readyNodes int32) (podsToUpgrade []corev1.Pod, err error) {
	queryParams := url.Values{}
	queryParams.Add("action", "CLUSTERSTATUS")
	clusterResp := &solr_api.SolrClusterStatusResponse{}
	overseerResp := &solr_api.SolrOverseerStatusResponse{}

	// TODO: Make these configurable?
	maxShardReplicasDown := 1
	maxBatchNodeUpgradeSpec := .5

	// If the maxBatchNodeUpgradeSpec is passed as a decimal between 0 and 1, then calculate as a percentage of the number of nodes.
	maxBatchNodeUpgrade := int(maxBatchNodeUpgradeSpec)
	if maxBatchNodeUpgradeSpec > 0 && maxBatchNodeUpgradeSpec < 1 {
		maxBatchNodeUpgrade = int(math.Ceil(maxBatchNodeUpgradeSpec * float64(cloud.Status.Replicas)))
	}

	if readyNodes > 0 {
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
		nodeContents, shardReplicasNotActive := FindSolrNodeContents(clusterResp.ClusterStatus, overseerResp.Leader)
		sortNodePodsBySafety(outOfDatePods, nodeContents, cloud)
		podsToUpgrade = make([]corev1.Pod, 0)
		for _, pod := range outOfDatePods {
			isSafe := true
			nodeName := SolrNodeName(cloud, pod)
			nodeContent, isInClusterState := nodeContents[nodeName]
			fmt.Printf("%s: %t %+v\n",pod.Name, isInClusterState, nodeContent)
			// The overseer can only be upgraded by itself
			if !isInClusterState {
				// All pods not in the cluster state are safe to upgrade
				isSafe = true
				fmt.Printf("%s: Killed. Reason - not in cluster state\n",pod.Name)
			} else if nodeContent.overseer {
				// The overseer can only be upgraded by itself
				// We want to update it when it's the last out of date pods and all other nodes are ready
				if len(outOfDatePods) == 1 && readyNodes == totalNodes {
					isSafe = true
					fmt.Printf("%s: Killed. Reason - overseer & last node\n",pod.Name)
				} else {
					isSafe = false
					fmt.Printf("%s: Saved. Reason - overseer\n",pod.Name)
				}
			} else {
				fmt.Printf("%s: Killed. Reason - does not conflict with totalReplicasPerShard\n",pod.Name)
				for shard, additionalReplicaCount := range nodeContent.totalReplicasPerShard {
					// If all of the replicas for a shard on the node are down, then this is safe to kill
					// TODO: Make sure this logic makes sense
					if additionalReplicaCount == nodeContent.downReplicasPerShard[shard] {
						 continue
					}

					notActiveReplicaCount, _ := shardReplicasNotActive[shard]

					// We have to allow killing of Pods that have multiple replicas of a shard
					// Therefore only check the additional Replica count if some replicas of that shard are already being upgraded
					// Also we only want to check the addition of the active replicas, as the non-active replicas are already included in the check.
					if notActiveReplicaCount > 0 && notActiveReplicaCount + nodeContent.activeReplicasPerShard[shard] > maxShardReplicasDown {
						isSafe = false
						break
					}
				}
			}
			if isSafe {
				for shard, additionalReplicaCount := range nodeContent.totalReplicasPerShard {
					shardReplicasNotActive[shard] += additionalReplicaCount
				}
				podsToUpgrade = append(podsToUpgrade, pod)

				// Stop after the maxBatchNodeUpgrade count, if one is provided.
				if maxBatchNodeUpgrade >= 1 && len(podsToUpgrade) == maxBatchNodeUpgrade {
					break
				}
			}
		}
	}
	return podsToUpgrade, err
}

func GroupPodsByNodeName(solrCloud *solr.SolrCloud, outOfDatePods []corev1.Pod) map[string]corev1.Pod {
	grouped := make(map[string]corev1.Pod, len(outOfDatePods))
	for _, pod := range outOfDatePods {
		grouped[SolrNodeName(solrCloud, pod)] = pod
	}
	return grouped
}

func sortNodePodsBySafety(outOfDatePods []corev1.Pod, nodeMap map[string]SolrNodeContents, solrCloud *solr.SolrCloud) {
	sort.Slice(outOfDatePods, func(i, j int) bool {
		nodeI, hasNodeI := nodeMap[SolrNodeName(solrCloud, outOfDatePods[i])]
		if !hasNodeI  {
			return true
		} else if nodeI.overseer {
			return false
		}
		nodeJ, hasNodeJ := nodeMap[SolrNodeName(solrCloud, outOfDatePods[i])]
		if !hasNodeJ  {
			return false
		} else if nodeJ.overseer {
			return true
		}
		return nodeI.leaders < nodeJ.leaders
	})
}

func FindSolrNodeContents(cluster solr_api.SolrClusterStatus, overseerLeader string) (nodeContents map[string]SolrNodeContents, shardReplicasNotActive map[string]int) {
	nodeContents = map[string]SolrNodeContents{}
	shardReplicasNotActive = map[string]int{}
	// Update the info for each "live" node.
	for _, nodeName := range cluster.LiveNodes {
		contents, hasValue := nodeContents[nodeName]
		if !hasValue {
			contents = SolrNodeContents{
				nodeName:              nodeName,
				leaders:               0,
				totalReplicasPerShard: map[string]int{},
				activeReplicasPerShard: map[string]int{},
				downReplicasPerShard:  map[string]int{},
				overseer:              false,
				live:                  true,
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
			for _, replica := range shard.Replicas {
				contents, hasValue := nodeContents[replica.NodeName]
				if !hasValue {
					contents = SolrNodeContents{
						nodeName:              replica.NodeName,
						leaders:               0,
						totalReplicasPerShard: map[string]int{},
						activeReplicasPerShard: map[string]int{},
						downReplicasPerShard:  map[string]int{},
						overseer:              false,
						live:                  false,
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
				if replica.State == solr_api.ReplicaDown || replica.State == solr_api.ReplicaRecoveryFailed  {
					contents.downReplicasPerShard[uniqueShard] += 1
				}

				nodeContents[replica.NodeName] = contents
			}
		}
	}
	// Update the info for the overseer leader.
	if overseerLeader != "" {
		contents, hasValue := nodeContents[overseerLeader]
		if !hasValue {
			contents = SolrNodeContents{
				nodeName:              overseerLeader,
				leaders:               0,
				totalReplicasPerShard: map[string]int{},
				activeReplicasPerShard: map[string]int{},
				downReplicasPerShard:  map[string]int{},
				overseer:              true,
				live:                  false,
			}
		} else {
			contents.overseer = true
		}
		nodeContents[overseerLeader] = contents
	}
	return nodeContents, shardReplicasNotActive
}

type SolrNodeContents struct {
	nodeName                string
	leaders                 int
	totalReplicasPerShard   map[string]int
	activeReplicasPerShard  map[string]int
	downReplicasPerShard    map[string]int
	overseer                bool
	live                    bool
}

func SolrNodeName(solrCloud *solr.SolrCloud, pod corev1.Pod) string {
	return fmt.Sprintf("%s:%d_solr", solrCloud.AdvertisedNodeHost(pod.Name), solrCloud.NodePort())
}