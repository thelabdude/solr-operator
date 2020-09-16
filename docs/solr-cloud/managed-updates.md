# Managed SolrCloud Rolling Updates

Solr Clouds are complex distributed systems, and thus require a more delicate and informed approach to rolling updates.

If the [`Managed` update strategy](solr-cloud-crd.md#update-strategy) is specified in the Solr Cloud CRD, then the Solr Operator will take control over deleting SolrCloud pods when they need to be updated.

The operator will find all pods that have not been udpated yet and choose the next set of pods to delete for an udpate, given the following workflow.

## Pod Update Workflow

The logic goes as follows:

1. Find the pods that are not up-to-date
1. Retrieve the cluster state of the SolrCloud if there are any `ready` pods.
    - If no pods are ready, then there is no endpoint to retrieve the cluster state from.
1. Sort the pods in order of saftey for being restarted. [Sorting order reference](#pod-update-sorting-order)
1. Iterate through the sorted pods, greedily choosing which pods to update. [Selection logic reference](#pod-update-selection-logic)
    

### Pod Update Sorting Order

The pods are sorted by the following criteria, in the given order.
If any two pods on a criteria, then the next criteria (in the following order) is used to sort them.

In this context the pods sorted highest are the first chosen to be updated, the pods sorted lowest will be selected last.

1. If the pod is the overseer, it will be sorted lowest.
1. If the pod is not represented in the clusterState, it will be sorted highest.
1. Number of leader replicas hosted in the pod, sorted low -> high
1. Number of replicas hosted in the pod, sorted low -> high
1. If the pod is not a liveNode, then it will be sorted lower.
1. Any pods that are equal on the above criteria will be sorted lexicographically.

### Pod Update Selection Logic

Loop over the sorted pods, until the number of pods selected to be updated has reached the maximum.
This maximum is calculated by taking the given, or default, [`maxPodsUnavailable`](solr-cloud-crd.md#update-strategy) and subtracting the number of updated pods that are unavailable or have yet to be re-created.
   1. If the pod is the overseer, then all other pods must be updated and available. If these conditions are not met, then the overseer pod cannot be udpated.
   1. If the pod contains no replicas, it is chosen to be updated.  
   **WARNING**: If you use Solr worker nodes for streaming expressions, you will likely want to set [`maxPodsUnavailable`](solr-cloud-crd.md#update-strategy) to a value you are comfortable with.
   1. If the taking down the replicas hosted in the pod would not violate the given [`maxShardReplicasUnavailable`](solr-cloud-crd.md#update-strategy), then the pod can be updated.
   Once a pod with replicas has been chosen to be updated, the replicas hosted in that pod are then considered unavailable for the rest of the selection logic.
   