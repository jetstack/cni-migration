cni-migration is a CLI tool for migrating a Kubernetes cluster's CNI solution
from Flannel (Canal) to Cilium. The tool works by running both CNIs at the same
time using [multus-cni](https://github.com/intel/multus-cni/). All pods are
updated to attach a network interface from both CNIs, and then migrate each node
to only running Cilium. This ensures that all pods are able to communicate to
both networks at all times during the migration.

## How

The following are the steps taken to migrate the CNI. During and after each
step, the inter-pod communication is regularly tested using
[knet-stress](https://github.com/joshvanl/knet-stress), which will send a HTTP
request to all other knet-stress instances on all nodes. This proves a
bi-directional network connectivity across cluster.

1. This step involves installing both CNIs on all nodes and labelling the nodes
   accordingly.

- Label all nodes with `node-role.kubernetes/canal-cilium=true` and
  patch the canal DaemonSet to have a node selector on this label.
- Label all nodes with `node-role.kubernetes/cni-priority-canal=true`.
- Deploy two knet-stress DaemonSets that run two knet-stress instances on each
  node.
- Deploy two DeamonSet instances of Cilium.
  - The first has a node selector on the label
    `node-role.kubernetes/cilium-canal=true` and writes its CNI config
    to `99-cilium.conf`. This then runs on all nodes.
  - The second has a node selector on the label
    `node-role.kubernetes/cilium=true` and writes its CNI config
    to `00-cilium.conf`. This will not run until a node is being migrated.
- Deploy twp DaemonSet instances of Multus.
  - Deploy multus DaemonSet with the node
    selector`node-role.kubernetes/cni-priority-canal=true`. This has a static
    config that uses the Flannel CNI config for the main Pod IP network interface, and
    the Cilium as an extra network interface attached. The resulting CNI config is
    written to `00-multus.conflist`. This CNI config will be chosen by the Kubelet
    until the node has been migrated.
  - Deploy multus DaemonSet with the node
    selector`node-role.kubernetes/cni-priority-cilium=true`. This multus is the
    same as the previous however swaps the primary Pod IP to that of Cilium
    rather than Flannel.

2. This step ensures that all workloads on the cluster are running with network
   interfaces from both CNIs. The "sbr" Channing CNI is used to the at the
   default route inside each pod is Cilium, however the Pod IP remains that of
   the range of Flannel.

- Roll all nodes in the cluster one by one. This step ensures that every pod
  in the cluster is reassigned an IP, meaning that all pods will have a
  network interface from both CNIs applied using multus.
- Check knet-stress connectively after every node roll.
- At this stage, all pods on the cluster have both CNI network interfaces
  attached. All nodes are running the two CNIs which are controlled by multus.

3. This step will reverse the order of priority of CNIs, so that Cilium becomes
   the primary Pod IP, with an extra Flannel network interface attached.

- Relabel and roll all the nodes on the on the cluster with the label
  `node-role.kubernetes/cni-priority-cilium=true`. This will change the priority
   of the CNI on each cluster to Cilium and have each Pod IP be in Cilium's range.
- Check knet-stress connectively after every node roll.
- At this stage, all pods on the cluster have both CNI network interfaces
  attached, however the Pod IP is not in Cilium's range, rather than Flannel.

4. This step is iterative by performing the same operation on all nodes until
   they have all been migrated.

- First, the selected node is drained, tainted, and has all pods deleted on it.
  This node removes the label `node-role.kubernetes/cilium-canal=true`.
  The taint added uses the label `node-role.kubernetes/cilium=true` which
  terminates the first Cilium DaemonSet, replaced with the second. This second
  Cilium DaemonSet writes its CNI config to `00-cilium.conf` which puts it as
  the first CNI config to be selected and used by Kubelet, making this node now
  only use Cilium CNI, rather than multus (Cilium _and_ Canal).
- The node is untainted which allows workloads to be re-scheduled to it,
  which will have only Cilium CNI network interfaces attached. These pods should
  still be reachable by all other pods in the cluster.
- The node has the label `node-role.kubernetes/migrated=true` added which
  signals that this node has been migrated.

5. After migrating all nodes, we now do a simple clean up of old resources.

- The now unused and non-scheduled Multus, Canal, and first Cilium DaemonSets
  are deleted.

The cluster should now be fully migrated from Canal to Cilium CNI.

## Requirements

The following requirements apply in order to run the migration.

### Firewall

- Cilium uses Geneve as a backend mode and as such, needs the port 6081 over UDP
  to communicate across nodes. This must be opened before migration.
  *Note*: Cilium can not run in VXLAN mode since it has not been possible to
  run two separate VXLAN interfaces on each host (one for Flannel and one for
  Cilium).
- All Kubernetes NetworkPolices will remain active and applied during, and after
  the migration, being compatible with Cilium. No action needed.

### Images

- docker.io/cilium/cilium:v1.7.2
- docker.io/cilium/operator:v1.7.2
- nfvpe/multus:v3.4.1
- gcr.io/jetstack-josh/knet-stress:cli (preferably a private image is built from
  source and used)

## Configuration

The cni-migration tool has input configuration file (default `--config
conifg.yaml`), that holds options for the migration.

### labels

This holds options on which label keys and shared value should be used for each
signal of steps:

```yaml
  canal-cilium: node-role.kubernetes.io/canal-cilium
  cni-priority-canal: node-role.kubernetes.io/priority-canal
  cni-priority-cilium: node-role.kubernetes.io/priority-cilium
  rolled: node-role.kubernetes.io/rolled
  cilium: node-role.kubernetes.io/cilium
  migrated: node-role.kubernetes.io/migrated
  value: "true" # used as the value to each label key
```

### paths

The file paths for each manifest bundle:

```yaml
  cilium: ./resources/cilium.yaml
  multus: ./resources/multus.yaml
  knet-stress: ./resources/knet-stress.yaml
```

### preflightResources

List of resources that must exist before beginning the migration.

```yaml
  daemonsets:
    knet-stress:
    - knet-stress
    - knet-stress-2
  deployments:
  statefulsets:
```

### watchedResources

List of resources which must be ready when checked throughout the migration
before continuing:

```yaml
  daemonsets:
    kube-system:
    - canal
    - cilium
    - cilium-migrated
    - kube-multus-canal
    - kube-multus-cilium
    - kube-controller-manager
    - kube-scheduler
    knet-stress:
    - knet-stress
    - knet-stress-2
  deployments:
  statefulsets:
```

### cleanUpResources

List of resources which will be removed after completing the migration
successfully:

```yaml
  daemonsets:
    kube-system:
    - canal
    - cilium
    - kube-multus-canal
    - kube-multus-cilium
    knet-stress:
    - knet-stress
    - knet-stress-2
  deployments:
  statefulsets:
```
