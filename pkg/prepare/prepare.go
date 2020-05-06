package prepare

import (
	"context"

	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/joshvanl/cni-migration/pkg/types"
	"github.com/joshvanl/cni-migration/pkg/util"
)

var _ types.Step = &Prepare{}

type Prepare struct {
	client  *kubernetes.Clientset
	log     *logrus.Entry
	ctx     context.Context
	factory *util.Factory
}

func New(ctx context.Context, log *logrus.Entry, client *kubernetes.Clientset) types.Step {
	return &Prepare{
		log:     log.WithField("step", "1-prepare"),
		client:  client,
		ctx:     ctx,
		factory: util.New(log, ctx, client),
	}
}

// Ready ensures that
// - Nodes have correct labels
// - The required resources exist
// - Canal DaemonSet has been patched
func (p *Prepare) Ready() (bool, error) {
	nodes, err := p.client.CoreV1().Nodes().List(p.ctx, metav1.ListOptions{})
	if err != nil {
		return false, err
	}

	for _, n := range nodes.Items {
		if !hasRequiredLabel(n.Labels) {
			return false, nil
		}
	}

	patched, err := p.canalIsPatched()
	if err != nil || !patched {
		return false, err
	}

	requiredResources, err := p.hasRequiredResources()
	if err != nil || !requiredResources {
		return false, err
	}

	if err := p.factory.CheckKnetStress(); err != nil {
		return false, err
	}

	return true, nil
}

// Run will ensure that
// - Node have correct labels
// - The required resources exist
// - Canal DaemonSet has been patched
func (p *Prepare) Run(dryrun bool) error {
	p.log.Infof("preparing migration...")

	nodes, err := p.client.CoreV1().Nodes().List(p.ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, n := range nodes.Items {
		if !hasRequiredLabel(n.Labels) {
			p.log.Infof("updating label on node %s", n.Name)

			if dryrun {
				continue
			}

			delete(n.Labels, types.LabelCiliumKey)
			n.Labels[types.LabelCanalCiliumKey] = types.LabelCanalCiliumValue

			_, err := p.client.CoreV1().Nodes().Update(p.ctx, n.DeepCopy(), metav1.UpdateOptions{})
			if err != nil {
				return err
			}
		}
	}

	patched, err := p.canalIsPatched()
	if err != nil {
		return err
	}

	if !patched {
		p.log.Infof("patching canal DaemonSet with node selector %s=%s",
			types.LabelCanalCiliumKey, types.LabelCanalCiliumValue)

		if !dryrun {
			if err := p.patchCanal(); err != nil {
				return err
			}
		}
	}

	requiredResources, err := p.hasRequiredResources()
	if err != nil {
		return err
	}

	if !requiredResources {
		// Create needed resources
		p.log.Infof("creating knet-stress resources")
		if !dryrun {
			if err := p.factory.CreateDaemonSet(types.PathKnetStress, "knet-stress", "knet-stress"); err != nil {
				return err
			}
		}

		p.log.Infof("creating cilium resources")
		if !dryrun {
			if err := p.factory.CreateDaemonSet(types.PathCilium, "kube-system", "cilium"); err != nil {
				return err
			}
		}

		p.log.Infof("creating multus resources")
		if !dryrun {
			if err := p.factory.CreateDaemonSet(types.PathMultus, "kube-system", "kube-multus-ds-amd64"); err != nil {
				return err
			}
		}
	}

	if !dryrun {
		if err := p.factory.WaitAllReady(); err != nil {
			return err
		}
	}

	if err := p.factory.CheckKnetStress(); err != nil {
		return err
	}

	return nil
}

func (p *Prepare) patchCanal() error {
	ds, err := p.client.AppsV1().DaemonSets("kube-system").Get(p.ctx, "canal", metav1.GetOptions{})
	if err != nil {
		return err
	}

	if ds.Spec.Template.Spec.NodeSelector == nil {
		ds.Spec.Template.Spec.NodeSelector = make(map[string]string)
	}
	ds.Spec.Template.Spec.NodeSelector[types.LabelCanalCiliumKey] = types.LabelCanalCiliumValue

	_, err = p.client.AppsV1().DaemonSets("kube-system").Update(p.ctx, ds, metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	return nil
}

func hasRequiredLabel(labels map[string]string) bool {
	if labels == nil {
		return false
	}

	_, cclOK := labels[types.LabelCanalCiliumKey]
	_, clOK := labels[types.LabelCiliumKey]

	// If both true, or both false, does not have correct labels
	if cclOK == clOK {
		return false
	}

	return true
}

func (p *Prepare) canalIsPatched() (bool, error) {
	ds, err := p.client.AppsV1().DaemonSets("kube-system").Get(p.ctx, "canal", metav1.GetOptions{})
	if err != nil {
		return false, err
	}

	if ds.Spec.Template.Spec.NodeSelector == nil {
		return false, nil
	}
	if v, ok := ds.Spec.Template.Spec.NodeSelector[types.LabelCanalCiliumKey]; !ok || v != types.LabelCanalCiliumValue {
		return false, nil
	}

	return true, nil
}

func (p *Prepare) hasRequiredResources() (bool, error) {
	for _, name := range types.DaemonSetNames {
		_, err := p.client.AppsV1().DaemonSets("kube-system").Get(p.ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
	}

	return true, nil
}
