cni-migration is a CLI tool for migrating a Kubernetes cluster's CNI solution
from Flannel (Canal) to Cilium. The tool works by running both CNIs at the same
time using [multus-cni](https://github.com/intel/multus-cni/). All pods are
updated to attach a network interface from both CNIs, and then migrate each node
to running only Cilium. This ensures that all pods are able to communicate to
both networks at all times during the migration.

## How

The following are the steps taken to migrate the CNI. During and after each
step, the inter-pod communication is regularly tested using knet-stress, which
will send an HTTP request to all other knet-stress instances on all nodes. This
proves a bi-directional network connectivity across cluster.

1. This step involves installing both CNIs on all nodes and labelling the nodes
   accordingly.

- Label all nodes with `node-role.kubernetes/cilium-canal=cilium-canal` and
  patch the canal DaemonSet to have a node selector on this label.
- Deploy two knet-stress DaemonSets that run two knet-stress instances on each
  node.
- Deploy two DeamonSet instances of Cilium.
  - The first has a node selector on the label
    `node-role.kubernetes/cilium-canal=cilium-canal` and writes its CNI config
    to `99-cilium.conf`. This then runs on all nodes.
  - The second has a node selector on the label
    `node-role.kubernetes/cilium=cilium` and writes its CNI config
    to `00-cilium.conf`. This will not run until a node is being migrated.
- Deploy multus DaemonSet with the node
  selector`node-role.kubernetes/cilium-canal=cilium-canal`. This has a static
  config that uses the Canal CNI config for the main Pod IP network inteterface,
  and the Cilium as an extra network interface attached. The resulting CNI
  config is written to `00-multus.config`. This CNI config will be chosen by the
  Kubelet until the node has been migrated.

2. This step ensures that all workloads on the cluster are running with network
   interfaces from both CNIs.

- Roll all nodes in the cluster one by one. This step ensures that every pod
  in the cluster is reassigned an IP, meaning that all pods will have a
  network interface from both CNIs applied using multus.
- Check knet-stress connectively after every node roll.
- At this stage, all pods on the cluster have both CNI network interfaces
  attached. All nodes are running the two CNIs which are controlled by multus.

3. This step is iterative by performing the same operation on all nodes until
   they have all been migrated.

- First, the selected node is drained, tainted, and has all pods deleted on it.
  This node removes the label `node-role.kubernetes/cilium-canal=cilium-canal`.
  The taint added uses the label `node-role.kubernetes/cilium=cilium` which
  terminates the first Cilium DaemonSet, replaced with the second. This second
  Cilium DaemonSet writes its CNI config to `00-cilium.conf` which puts it as
  the first CNI config to be selected and used by Kubelet, making this node now
  only use Cilium CNI, rather than multus (Cilium _and_ Canal).
- The node is untainted which allows workloads to be re-scheduled to it,
  which will have only Cilium CNI network interfaces attached. These pods should
  still be reachable by all other pods in the cluster.
- The node has the label `node-role.kubernetes/migrated=migrated` added which
  signals that this node has been migrated.

4. After migrating all nodes, we now do a simple clean up of old resources.

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
