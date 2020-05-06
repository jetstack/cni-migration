package migrate

import (
	"context"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/joshvanl/cni-migration/pkg/types"
	"github.com/joshvanl/cni-migration/pkg/util"
)

var _ types.Step = &Migrate{}

type Migrate struct {
	client  *kubernetes.Clientset
	log     *logrus.Entry
	ctx     context.Context
	factory *util.Factory
}

func New(ctx context.Context, log *logrus.Entry, client *kubernetes.Clientset) types.Step {
	return &Migrate{
		log:     log.WithField("step", "3-migrate"),
		client:  client,
		ctx:     ctx,
		factory: util.New(log, ctx, client),
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
		if !hasRequiredLabel(n.Labels) {
			return false, nil
		}
	}

	if err := m.factory.CheckKnetStress(); err != nil {
		return false, err
	}

	return true, nil
}

func (m *Migrate) Run(dryrun bool) error {
	m.log.Info("migrating nodes...")

	nodes, err := m.client.CoreV1().Nodes().List(m.ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, n := range nodes.Items {
		if !hasRequiredLabel(n.Labels) {
			if err := m.node(dryrun, n.Name); err != nil {
				return err
			}

			if v := m.ctx.Value(types.ContextSingleNodeKey); v == "true" {
				break
			}
		}
	}

	return nil
}

func (m *Migrate) node(dryrun bool, nodeName string) error {
	m.log.Infof("%s Draining node", nodeName)

	if !dryrun {
		args := []string{"kubectl", "drain", "--delete-local-data", "--ignore-daemonsets", nodeName}
		if err := m.factory.RunCommand(args...); err != nil {
			return err
		}

		args = []string{"kubectl", "taint", "node", nodeName, "node-role.kubernetes.io/cilium=cilium:NoExecute", "--overwrite"}
		if err := m.factory.RunCommand(args...); err != nil {
			return err
		}
	}

	// Add taint on node
	m.log.Infof("%s Adding %s=%s:NoExecute taint", nodeName, types.LabelCiliumKey, types.LabelCiliumValue)
	if !dryrun {
		if err := m.addCiliumTaint(nodeName); err != nil {
			return err
		}

		// Delete all pods on that node
		if err := m.factory.DeletePodsOnNode(nodeName); err != nil {
			return err
		}

		if err := m.factory.WaitAllReady(); err != nil {
			return err
		}

		// Check knet connectivity
		if err := m.factory.CheckKnetStress(); err != nil {
			return err
		}
	}

	// Remove taint on node
	m.log.Infof("%s Removing %s=%s:NoExecute taint", nodeName, types.LabelCiliumKey, types.LabelCiliumValue)
	if !dryrun {
		if err := m.deleteCiliumTaint(nodeName); err != nil {
			return err
		}
	}

	m.log.Infof("%s Uncordoning node", nodeName)
	if !dryrun {
		args := []string{"kubectl", "uncordon", nodeName}
		if err := m.factory.RunCommand(args...); err != nil {
			return err
		}

		if err := m.factory.WaitAllReady(); err != nil {
			return err
		}
	}

	m.log.Infof("%s Adding label %s=true", nodeName, types.LabelMigratedKey)
	if !dryrun {
		if err := m.setNodeMigratedLabel(nodeName); err != nil {
			return err
		}
	}

	if err := m.factory.CheckKnetStress(); err != nil {
		return err
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
		if t.Key != types.LabelCiliumKey {
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
		if t.Key == types.LabelCiliumKey {
			hasTaint = true
			break
		}
	}

	if !hasTaint {
		node.Spec.Taints = append(node.Spec.Taints, corev1.Taint{
			Key:    types.LabelCiliumKey,
			Value:  types.LabelCiliumValue,
			Effect: corev1.TaintEffectNoExecute,
		})
	}

	// Change label of node
	delete(node.Labels, types.LabelCanalCiliumKey)
	node.Labels[types.LabelCiliumKey] = types.LabelCiliumValue

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
	delete(node.Labels, types.LabelCanalCiliumKey)
	node.Labels[types.LabelMigratedKey] = "true"

	_, err = m.client.CoreV1().Nodes().Update(m.ctx, node, metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	return nil
}

func hasRequiredLabel(labels map[string]string) bool {
	if labels == nil {
		return false
	}

	if _, ok := labels[types.LabelMigratedKey]; !ok {
		return false
	}

	return true
}
