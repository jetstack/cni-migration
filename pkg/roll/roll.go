package roll

import (
	"context"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/joshvanl/cni-migration/pkg"
	"github.com/joshvanl/cni-migration/pkg/config"
	"github.com/joshvanl/cni-migration/pkg/util"
)

var _ pkg.Step = &Roll{}

type Roll struct {
	ctx context.Context
	log *logrus.Entry

	config  *config.Config
	client  *kubernetes.Clientset
	factory *util.Factory
}

func New(ctx context.Context, config *config.Config) pkg.Step {
	log := config.Log.WithField("step", "2-roll")
	return &Roll{
		log:     log,
		ctx:     ctx,
		config:  config,
		client:  config.Client,
		factory: util.New(ctx, log, config.Client),
	}
}

// Ready ensures that
// - All nodes have the 'rolled' label
func (r *Roll) Ready() (bool, error) {
	nodes, err := r.client.CoreV1().Nodes().List(r.ctx, metav1.ListOptions{})
	if err != nil {
		return false, err
	}

	for _, n := range nodes.Items {
		if !r.hasRequiredLabel(n.Labels) {
			return false, nil
		}
	}

	r.log.Info("step 2 ready")

	return true, nil
}

func (r *Roll) Run(dryrun bool) error {
	r.log.Info("rolling nodes to install multi CNI...")

	if !dryrun {
		if err := r.factory.CheckKnetStress(); err != nil {
			return err
		}
	}

	nodes, err := r.client.CoreV1().Nodes().List(r.ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, n := range nodes.Items {
		if !r.hasRequiredLabel(n.Labels) {
			r.log.Infof("rolling node: %s", n.Name)

			if err := r.node(dryrun, n.Name); err != nil {
				return err
			}
		}
	}

	return nil
}

func (r *Roll) node(dryrun bool, name string) error {
	r.log.Infof("draining node %s", name)

	node, err := r.client.CoreV1().Nodes().Get(r.ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	r.log.Infof("Draining node %s", name)
	if !dryrun {
		args := []string{"kubectl", "drain", "--delete-local-data", "--ignore-daemonsets", name}
		if err := r.factory.RunCommand(args...); err != nil {
			return err
		}

		if err := r.factory.WaitAllReady(r.config.WatchedResources); err != nil {
			return err
		}
	}

	// Delete all pods on that node
	r.log.Infof("Deleting all pods on node %s", name)
	if !dryrun {
		if err := r.factory.DeletePodsOnNode(name); err != nil {
			return err
		}
	}

	r.log.Infof("Uncordoning node %s", name)
	if !dryrun {
		args := []string{"kubectl", "uncordon", name}
		if err := r.factory.RunCommand(args...); err != nil {
			return err
		}

		if err := r.factory.WaitAllReady(r.config.WatchedResources); err != nil {
			return err
		}

		if err := r.factory.CheckKnetStress(); err != nil {
			return err
		}
	}

	node, err = r.client.CoreV1().Nodes().Get(r.ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	r.log.Infof("Adding rolled label to node %s", name)
	if !dryrun {
		if node.Labels == nil {
			node.Labels = make(map[string]string)
		}
		node.Labels[r.config.Labels.Rolled] = r.config.Labels.Value

		_, err = r.client.CoreV1().Nodes().Update(r.ctx, node, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *Roll) hasRequiredLabel(labels map[string]string) bool {
	if labels == nil {
		return false
	}

	if v, ok := labels[r.config.Labels.Rolled]; !ok || v != r.config.Labels.Value {
		return false
	}

	return true
}
