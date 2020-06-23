package migrate

import (
	"context"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/joshvanl/cni-migration/pkg"
	"github.com/joshvanl/cni-migration/pkg/config"
	"github.com/joshvanl/cni-migration/pkg/util"
)

const (
	ContextNodesKey = "cni-migration-migrate-nodes"
)

var _ pkg.Step = &Migrate{}

type Migrate struct {
	ctx context.Context
	log *logrus.Entry

	config  *config.Config
	client  *kubernetes.Clientset
	factory *util.Factory
}

func New(ctx context.Context, config *config.Config) pkg.Step {
	log := config.Log.WithField("step", "4-migrate")
	return &Migrate{
		log:     log,
		ctx:     ctx,
		config:  config,
		client:  config.Client,
		factory: util.New(ctx, log, config.Client),
	}
}

// Ready ensures that
// - All nodes have the 'migrated' label
func (m *Migrate) Ready() (bool, error) {
	nodes, err := m.client.CoreV1().Nodes().List(m.ctx, metav1.ListOptions{})
	if err != nil {
		return false, err
	}

	for _, n := range nodes.Items {
		if !m.hasRequiredLabel(n.Labels) {
			return false, nil
		}
	}

	m.log.Info("step 4 ready")

	return true, nil
}

func (m *Migrate) Run(dryrun bool) error {
	nodes, flagEnabled, err := util.NodesFromContext(m.client, m.ctx, ContextNodesKey)
	if err != nil {
		return err
	}

	if !flagEnabled {
		m.log.Info("migrating all nodes...")

		nodesList, err := m.client.CoreV1().Nodes().List(m.ctx, metav1.ListOptions{})
		if err != nil {
			return err
		}

		nodes = nodesList.Items
	}

	for _, node := range nodes {
		m.log.Infof("migrating nodes %s...", node.Name)

		if !m.hasRequiredLabel(node.Labels) {
			if err := m.node(dryrun, node.Name); err != nil {
				return err
			}
		}
	}

	return nil
}

func (m *Migrate) node(dryrun bool, nodeName string) error {
	m.log.Infof("Draining node %s", nodeName)

	if !dryrun {
		if err := m.factory.CheckKnetStress(); err != nil {
			return err
		}

		args := []string{"kubectl", "drain", "--delete-local-data", "--ignore-daemonsets", nodeName}
		if err := m.factory.RunCommand(nil, args...); err != nil {
			return err
		}

		args = []string{"kubectl", "taint", "node", nodeName, "node-role.kubernetes.io/cilium=cilium:NoExecute", "--overwrite"}
		if err := m.factory.RunCommand(nil, args...); err != nil {
			return err
		}
	}

	// Add taint on node
	m.log.Infof("Adding %s=%s:NoExecute taint to node %s ",
		m.config.Labels.Cilium, m.config.Labels.Value, nodeName)
	if !dryrun {
		if err := m.addCiliumTaint(nodeName); err != nil {
			return err
		}
	}

	m.log.Infof("removing pods on node %s", nodeName)
	if !dryrun {

		if err := m.factory.WaitDaemonSetReady("kube-system", "cilium-migrated"); err != nil {
			return err
		}

		// Delete all pods on that node
		if err := m.factory.DeletePodsOnNode(nodeName); err != nil {
			return err
		}

		if err := m.factory.WaitAllReady(m.config.WatchedResources); err != nil {
			return err
		}

		// Check knet connectivity
		if err := m.factory.CheckKnetStress(); err != nil {
			return err
		}
	}

	// Remove taint on node
	m.log.Infof("removing %s=%s:NoExecute taint on node %s",
		m.config.Labels.Cilium, m.config.Labels.Value, nodeName)
	if !dryrun {
		if err := m.deleteCiliumTaint(nodeName); err != nil {
			return err
		}
	}

	m.log.Infof("uncordoning node %s", nodeName)
	if !dryrun {
		args := []string{"kubectl", "uncordon", nodeName}
		if err := m.factory.RunCommand(nil, args...); err != nil {
			return err
		}

		if err := m.factory.WaitAllReady(m.config.WatchedResources); err != nil {
			return err
		}
	}

	m.log.Infof("adding label %s=% to node %s",
		m.config.Labels.Migrated, m.config.Labels.Value, nodeName)
	if !dryrun {
		if err := m.setNodeMigratedLabel(nodeName); err != nil {
			return err
		}

		if err := m.factory.CheckKnetStress(); err != nil {
			return err
		}
	}

	return nil
}

func (m *Migrate) deleteCiliumTaint(nodeName string) error {
	node, err := m.client.CoreV1().Nodes().Get(m.ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	var taints []corev1.Taint
	for _, t := range node.Spec.Taints {
		if t.Key != m.config.Labels.Cilium {
			taints = append(taints, t)
		}
	}
	node.Spec.Taints = taints

	_, err = m.client.CoreV1().Nodes().Update(m.ctx, node, metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	return nil
}

func (m *Migrate) addCiliumTaint(nodeName string) error {
	node, err := m.client.CoreV1().Nodes().Get(m.ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	hasTaint := false
	for _, t := range node.Spec.Taints {
		if t.Key == m.config.Labels.Cilium {
			hasTaint = true
			break
		}
	}

	if !hasTaint {
		node.Spec.Taints = append(node.Spec.Taints, corev1.Taint{
			Key:    m.config.Labels.Cilium,
			Value:  m.config.Labels.Value,
			Effect: corev1.TaintEffectNoExecute,
		})
	}

	// Change label of node
	delete(node.Labels, m.config.Labels.CanalCilium)
	node.Labels[m.config.Labels.Cilium] = m.config.Labels.Value

	node, err = m.client.CoreV1().Nodes().Update(m.ctx, node, metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	return nil
}

func (m *Migrate) setNodeMigratedLabel(nodeName string) error {
	node, err := m.client.CoreV1().Nodes().Get(m.ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	// Set migrated label
	delete(node.Labels, m.config.Labels.CNIPriorityCilium)
	delete(node.Labels, m.config.Labels.CanalCilium)
	node.Labels[m.config.Labels.Migrated] = m.config.Labels.Value

	_, err = m.client.CoreV1().Nodes().Update(m.ctx, node, metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	return nil
}

func (m *Migrate) hasRequiredLabel(labels map[string]string) bool {
	if labels == nil {
		return false
	}

	if v, ok := labels[m.config.Labels.Migrated]; !ok || v != m.config.Labels.Value {
		return false
	}

	if v, ok := labels[m.config.Labels.CNIPriorityCilium]; ok && v == m.config.Labels.Value {
		return false
	}

	return true
}
