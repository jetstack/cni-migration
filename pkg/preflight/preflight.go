package preflight

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/joshvanl/cni-migration/pkg"
	"github.com/joshvanl/cni-migration/pkg/config"
	"github.com/joshvanl/cni-migration/pkg/util"
)

var _ pkg.Step = &Preflight{}

type Preflight struct {
	ctx    context.Context
	config *config.Config

	log     *logrus.Entry
	factory *util.Factory
}

func New(ctx context.Context, config *config.Config) pkg.Step {
	log := config.Log.WithField("step", "0-preflight")
	return &Preflight{
		ctx:     ctx,
		log:     log,
		config:  config,
		factory: util.New(ctx, log, config.Client),
	}
}

// Ready ensures that
// - Knet-stress is running
// - Knet-stress is healthy
func (p *Preflight) Ready() (bool, error) {
	requiredResources, err := p.factory.Has(p.config.PreflightResources)
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

	requiredResources, err := p.factory.Has(p.config.PreflightResources)
	if err != nil {
		return err
	}

	if !requiredResources {
		p.log.Infof("creating knet-stress resources")
		if !dryrun {
			if err := p.factory.CreateDaemonSet(p.config.Paths.KnetStress, "knet-stress", "knet-stress"); err != nil {
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
