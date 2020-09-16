# The SolrCloud CRD

The SolrCloud CRD allows users to spin up a Solr cloud in a very configurable way.
Those configuration options are layed out on this page.

## Update Strategy

The SolrCloud CRD provides users the ability to define how it is addressed, through `SolrCloud.Spec.updateStrategy`.
This provides the following options:

Under `SolrCloud.Spec.updateStrategy`:

- **`method`** - The method in which Solr pods should be updated. Enum options are as follows:
  - `Managed` - (Default) The Solr Operator will take control over deleting pods for updates. This process is [documented here](managed-updates.md).
  - `StatefulSet` - Use the default StatefulSet rolling update logic, one pod at a time waiting for all pods to be "ready".
  - `Manual` - Neither the StatefulSet or the Solr Operator will delete pods in need of an update. The user will take responsibility over this.
- **`managed`** - Options for rolling updates managed by the Solr Operator.
  - **`maxPodsUnavailable`** - (Defaults to `"25%"`) The number of Solr pods in a Solr Cloud that are allowed to be unavailable during the rolling restart.
  More pods may become unavailable during the restart, however the Solr Operator will not kill pods if the limit has already been reached.  
  - **`maxShardReplicasUnavailable`** - (Defaults to `1`) The number of replicas for each shard allowed to be unavailable during the restart.
  
**Note:** Both `maxPodsUnavailable` and `maxShardReplicasUnavailable` are intOrString fields. So either an int or string can be provided for the field.
- **int** - The parameter is treated as an absolute value, unless the value is <= 0 which is interpreted as unlimited.
- **string** - Only percentage string values (`"0%"` - `"100%"`) are accepted, all other values will be ignored.
  - **`maxPodsUnavailable`** - The `maximumPodsUnavailable` is calculated as the percentage of the total pods configured for that Solr Cloud.
  - **`maxShardReplicasUnavailable`** - The `maxShardReplicasUnavailable` is calculated independently for each shard, as the percentage of the number of replicas for that shard.

## Addressability

The SolrCloud CRD provides users the ability to define how it is addressed, through the following options:

Under `SolrCloud.Spec.solrAddressability`:

- **`podPort`** - The port on which the pod is listening. This is also that the port that the Solr Jetty service will listen on. (Defaults to `8983`)
- **`commonServicePort`** - The port on which the common service is exposed. (Defaults to `80`)
- **`kubeDomain`** - Specifies an override of the default Kubernetes cluster domain name, `cluster.local`. This option should only be used if the Kubernetes cluster has been setup with a custom domain name.
- **`external`** - Expose the cloud externally, outside of the kubernetes cluster in which it is running.
  - **`method`** - (Required) The method by which your cloud will be exposed externally.
  Currently available options are [`Ingress`](https://kubernetes.io/docs/concepts/services-networking/ingress/) and [`ExternalDNS`](https://github.com/kubernetes-sigs/external-dns).
  The goal is to support more methods in the future, such as LoadBalanced Services.
  - **`domainName`** - (Required) The primary domain name to open your cloud endpoints on. If `useExternalAddress` is set to `true`, then this is the domain that will be used in Solr Node names.
  - **`additionalDomainNames`** - You can choose to listen on additional domains for each endpoint, however Solr will not register itself under these names.
  - **`useExternalAddress`** - Use the external address to advertise the SolrNode. If a domain name is required for the chosen external `method`, then the one provided in `domainName` will be used.
  - **`hideCommon`** - Do not externally expose the common service (one endpoint for all solr nodes).
  - **`hideNodes`** - Do not externally expose each node. (This cannot be set to `true` if the cloud is running across multiple kubernetes clusters)
  - **`nodePortOverride`** - Make the Node Service(s) override the podPort. This is only available for the `Ingress` external method. If `hideNodes` is set to `true`, then this option is ignored. If provided, his port will be used to advertise the Solr Node. \
  If `method: Ingress` and `hideNodes: false`, then this value defaults to `80` since that is the default port that ingress controllers listen on.

**Note:** Unless both `external.method=Ingress` and `external.hideNodes=false`, a headless service will be used to make each Solr Node in the statefulSet addressable.
If both of those criteria are met, then an individual ClusterIP Service will be created for each Solr Node/Pod.

## Zookeeper Reference

Solr Clouds require an Apache Zookeeper to connect to.

The Solr operator gives a few options.

**Note** - Both options below come with options to specify a `chroot`, or a ZNode path for solr to use as it's base "directory" in Zookeeper. Before the operator creates or updates a StatefulSet with a given `chroot`, it will first ensure that the given ZNode path exists and if it doesn't the operator will create all necessary ZNodes in the path. If no chroot is given, a default of `/` will be used, which doesn't require the existence check previously mentioned. If a chroot is provided without a prefix of `/`, the operator will add the prefix, as it is required by Zookeeper.

### ZK Connection Info

This is an external/internal connection string as well as an optional chRoot to an already running Zookeeeper ensemble.
If you provide an external connection string, you do not _have_ to provide an internal one as well.

### Provided Instance

If you do not require the Solr cloud to run cross-kube cluster, and do not want to manage your own Zookeeper ensemble,
the solr-operator can manage Zookeeper ensemble(s) for you.

#### Zookeeper

Using the [zookeeper-operator](https://github.com/pravega/zookeeper-operator), a new Zookeeper ensemble can be spun up for 
each solrCloud that has this option specified.

The startup parameter `zookeeper-operator` must be provided on startup of the solr-operator for this parameter to be available.

#### Zetcd

Using [etcd-operator](https://github.com/coreos/etcd-operator), a new Etcd ensemble can be spun up for each solrCloud that has this option specified.
A [Zetcd](https://github.com/etcd-io/zetcd) deployment is also created so that Solr can interact with Etcd as if it were a Zookeeper ensemble.

The startup parameter `etcd-operator` must be provided on startup of the solr-operator for this parameter to be available.
