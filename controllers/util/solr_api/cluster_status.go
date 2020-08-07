package solr_api

import solr "github.com/bloomberg/solr-operator/api/v1beta1"

type SolrOverseerStatusResponse struct {
	ResponseHeader SolrResponseHeader `json:"responseHeader"`

	// +optional
	Leader string `json:"leader"`

	// +optional
	QueueSize int `json:"overseer_queue_size"`

	// +optional
	WorkQueueSize int `json:"overseer_work_queue_size"`

	// +optional
	CollectionQueueSize int `json:"overseer_collection_queue_size"`
}

type SolrClusterStatusResponse struct {
	ResponseHeader SolrResponseHeader `json:"responseHeader"`

	// +optional
	ClusterStatus SolrClusterStatus `json:"cluster"`
}

type SolrClusterStatus struct {
	// +optional
	Collections map[string]SolrCollectionStatus `json:"collections"`

	// +optional
	Aliases map[string]string `json:"aliases"`

	// +optional
	Roles map[string][]string `json:"roles"`

	// +optional
	LiveNodes []string `json:"live_nodes"`
}

type SolrCollectionStatus struct {
	// +optional
	Shards map[string]SolrShardStatus `json:"shards"`

	// +optional
	ConfigName string `json:"configName"`

	// +optional
	ZnodeVersion string `json:"znodeVersion"`

	// +optional
	AutoAddReplicas string `json:"autoAddReplicas"`

	// +optional
	NrtReplicas int `json:"nrtReplicas"`

	// +optional
	TLogReplicas int `json:"tlogReplicas"`

	// +optional
	PullReplicas int `json:"pullReplicas"`

	// +optional
	MaxShardsPerNode string `json:"maxShardsPerNode"`

	// +optional
	ReplicationFactor string `json:"replicationFactor"`

	// +optional
	Router SolrCollectionRouter `json:"router"`
}

type SolrCollectionRouter struct {
	Name solr.CollectionRouterName `json:"name"`
}

type SolrShardStatus struct {
	// +optional
	Replicas map[string]SolrReplicaStatus `json:"replicas"`

	// +optional
	Range string `json:"range"`

	// +optional
	State SolrShardState `json:"state"`
}

type SolrShardState string

const (
	ShardActive         SolrShardState = "active"
	ShardDown           SolrShardState = "down"
)

type SolrReplicaStatus struct {
	State SolrReplicaState `json:"state"`

	Core string `json:"core"`

	NodeName string `json:"node_name"`

	BaseUrl string `json:"base_url"`

	Leader bool `json:"leader"`

	// +optional
	Type SolrReplicaType `json:"type"`
}

type SolrReplicaState string

const (
	ReplicaActive         SolrReplicaState = "active"
	ReplicaDown           SolrReplicaState = "down"
	ReplicaRecovering     SolrReplicaState = "recovering"
	ReplicaRecoveryFailed SolrReplicaState = "recovery_failed"
)

type SolrReplicaType string

const (
	NRT                   SolrReplicaType = "NRT"
	TLOG                  SolrReplicaType = "TLOG"
	PULL     SolrReplicaType = "PULL"
)