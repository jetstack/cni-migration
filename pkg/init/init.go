package init

import (
	"context"

	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/joshvanl/cni-migration/pkg/types"
	"github.com/joshvanl/cni-migration/pkg/util"
)

var _ types.Step = &Init{}

type Init struct {
	client  *kubernetes.Clientset
	log     *logrus.Entry
	ctx     context.Context
	factory *util.Factory
}

func New(ctx context.Context, log *logrus.Entry, client *kubernetes.Clientset) *Init {
	return &Init{
		log:     log.WithField("step", "1-init"),
		client:  client,
		ctx:     ctx,
		factory: util.New(log, ctx, client),
	}
}

// Ready ensures that
// - Nodes have correct labels
// - The required resources exist
// - Canal DaemonSet has been patched
func (i *Init) Ready() (bool, error) {
	nodes, err := i.client.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return false, err
	}

	for _, n := range nodes.Items {
		if !hasRequiredLabel(n.Labels) {
			return false, nil
		}
	}

	patched, err := i.canalIsPatched()
	if err != nil || !patched {
		return false, err
	}

	requiredResources, err := i.hasRequiredResources()
	if err != nil || !requiredResources {
		return false, err
	}

	// TODO: check knet
	return true, nil
}

// Run will ensure that
// - Node have correct labels
// - The required resources exist
// - Canal DaemonSet has been patched
func (i *Init) Run(dryrun bool) error {
	m.log.Infof("preparing migration...")

	nodes, err := i.client.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, n := range nodes.Items {
		if !hasRequiredLabel(n.Labels) {
			i.log.Infof("updating label on node %s", n.Name)

			if dryrun {
				continue
			}

			delete(n.Labels, types.LabelCiliumKey)
			n.Labels[types.LabelCanalCiliumKey] = types.LabelCanalCiliumValue

			_, err := i.client.CoreV1().Nodes().Update(i.ctx, n.DeepCopy(), metav1.UpdateOptions{})
			if err != nil {
				return err
			}
		}
	}

	patched, err := i.canalIsPatched()
	if err != nil {
		return err
	}

	if !patched {
		i.log.Infof("patching canal DaemonSet with node selector %s=%s",
			types.LabelCanalCiliumKey, types.LabelCanalCiliumValue)

		if !dryrun {
			if err := i.patchCanal(); err != nil {
				return err
			}
		}
	}

	requiredResources, err := i.hasRequiredResources()
	if err != nil {
		return err
	}

	if !requiredResources {
		// Create needed resources
		i.log.Infof("creating knet-stress resources")
		if !dryrun {
			if err := i.factory.CreateDaemonSet(types.PathKnetStress, "knet-stress", "knet-stress"); err != nil {
				return err
			}
		}

		i.log.Infof("creating cilium resources")
		if !dryrun {
			if err := i.factory.CreateDaemonSet(types.PathCilium, "kube-system", "cilium"); err != nil {
				return err
			}
		}

		i.log.Infof("creating multus resources")
		if !dryrun {
			if err := i.factory.CreateDaemonSet(types.PathMultus, "kube-system", "kube-multus-ds-amd64"); err != nil {
				return err
			}
		}
	}

	if !dryrun {
		if err := i.factory.WaitAllReady(); err != nil {
			return err
		}
	}

	// TODO: check knet

	return nil
}

func (i *Init) patchCanal() error {
	ds, err := i.client.AppsV1().DaemonSets("kube-system").Get(i.ctx, "canal", metav1.GetOptions{})
	if err != nil {
		return err
	}

	if ds.Spec.Template.Spec.NodeSelector == nil {
		ds.Spec.Template.Spec.NodeSelector = make(map[string]string)
	}
	ds.Spec.Template.Spec.NodeSelector[types.LabelCanalCiliumKey] = types.LabelCanalCiliumValue

	_, err = i.client.AppsV1().DaemonSets("kube-system").Update(i.ctx, ds, metav1.UpdateOptions{})
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

func (i *Init) canalIsPatched() (bool, error) {
	ds, err := i.client.AppsV1().DaemonSets("kube-system").Get(i.ctx, "canal", metav1.GetOptions{})
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

func (i *Init) hasRequiredResources() (bool, error) {
	for _, name := range types.DaemonSetNames {
		_, err := i.client.AppsV1().DaemonSets("kube-system").Get(i.ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
	}

	return true, nil
}
