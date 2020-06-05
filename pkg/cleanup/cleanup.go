package cleanup

import (
	"context"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/joshvanl/cni-migration/pkg"
	"github.com/joshvanl/cni-migration/pkg/config"
	"github.com/joshvanl/cni-migration/pkg/util"
)

var _ pkg.Step = &CleanUp{}

type CleanUp struct {
	ctx context.Context
	log *logrus.Entry

	config  *config.Config
	client  *kubernetes.Clientset
	factory *util.Factory
}

func New(ctx context.Context, config *config.Config) pkg.Step {
	log := config.Log.WithField("step", "4-cleanup")
	return &CleanUp{
		log:     log,
		ctx:     ctx,
		config:  config,
		client:  config.Client,
		factory: util.New(ctx, log, config.Client),
	}
}

// Ready ensures that
// - All migration resources have been cleaned up
func (c *CleanUp) Ready() (bool, error) {
	cleanUpResources, err := c.factory.Has(c.config.CleanUpResources)
	if err != nil || cleanUpResources {
		return !cleanUpResources, err
	}

	c.log.Info("step 4 ready")

	return true, nil
}

func (c *CleanUp) Run(dryrun bool) error {
	c.log.Info("Cleaning up...")

	c.log.Infof("deleting multus: %s", c.config.Paths.Multus)
	if !dryrun {
		if err := c.factory.DeleteResource(c.config.Paths.Multus, "kube-system"); err != nil {
			return err
		}
	}

	c.log.Info("deleting canal DaemonSet")
	if !dryrun {
		err := c.client.AppsV1().DaemonSets("kube-system").Delete(c.ctx, "canal", metav1.DeleteOptions{})
		if err != nil {
			return err
		}
	}

	c.log.Info("deleting cilium DaemonSet")
	if !dryrun {
		err := c.client.AppsV1().DaemonSets("kube-system").Delete(c.ctx, "cilium", metav1.DeleteOptions{})
		if err != nil {
			return err
		}
	}

	return nil
}
