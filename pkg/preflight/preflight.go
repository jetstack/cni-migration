package preflight

import (
	"context"

	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/joshvanl/cni-migration/pkg/types"
	"github.com/joshvanl/cni-migration/pkg/util"
)

var _ types.Step = &Preflight{}

type Preflight struct {
	client  *kubernetes.Clientset
	log     *logrus.Entry
	ctx     context.Context
	factory *util.Factory
}

func New(ctx context.Context, log *logrus.Entry, client *kubernetes.Clientset) types.Step {
	log = log.WithField("step", "0-preflight")
	return &Preflight{
		log:     log,
		client:  client,
		ctx:     ctx,
		factory: util.New(log, ctx, client),
	}
}

// Ready ensures that
// - Knet-stress is running
// - Knet-stress is healthy
func (p *Preflight) Ready() (bool, error) {
	requiredResources, err := p.hasRequiredResources()
	if err != nil || !requiredResources {
		return false, err
	}

	if err := p.factory.WaitDaemonSetReady("knet-stress", "knet-stress"); err != nil {
		return false, err
	}

	if err := p.factory.CheckKnetStress(); err != nil {
		return false, err
	}

	p.log.Info("step 0 ready")

	return true, nil
}

// Run will ensure that
// - Knet-stress is deployed
// - Knet-stress is healty
func (p *Preflight) Run(dryrun bool) error {
	p.log.Infof("running preflight checks...")

	requiredResources, err := p.hasRequiredResources()
	if err != nil {
		return err
	}

	if !requiredResources {
		p.log.Infof("creating knet-stress resources")
		if !dryrun {
			if err := p.factory.CreateDaemonSet(types.PathKnetStress, "knet-stress", "knet-stress"); err != nil {
				return err
			}
		}
	}

	if !dryrun {
		if err := p.factory.CheckKnetStress(); err != nil {
			return err
		}
	}

	return nil
}

func (p *Preflight) hasRequiredResources() (bool, error) {
	for name, namespace := range types.DaemonSetPreflightNames {
		_, err := p.client.AppsV1().DaemonSets(namespace).Get(p.ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
	}

	return true, nil
}
