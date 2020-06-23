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
	log := config.Log.WithField("step", "5-cleanup")
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

	ds, err := c.client.AppsV1().DaemonSets("kube-system").Get(c.ctx, "cilium-migrated", metav1.GetOptions{})
	if err != nil {
		return false, err
	}

	if _, ok := ds.Spec.Template.Spec.NodeSelector[c.config.Labels.Cilium]; ok {
		return false, nil
	}

	c.log.Info("step 5 ready")

	return true, nil
}

func (c *CleanUp) Run(dryrun bool) error {
	c.log.Info("cleaning up...")

	c.log.Info("removing node selector from cilium-migrated")
	if !dryrun {
		ds, err := c.client.AppsV1().DaemonSets("kube-system").Get(c.ctx, "cilium-migrated", metav1.GetOptions{})
		if err != nil {
			return err
		}

		delete(ds.Spec.Template.Spec.NodeSelector, c.config.Labels.Cilium)

		_, err = c.client.AppsV1().DaemonSets("kube-system").Update(c.ctx, ds, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
	}

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
