package cleanup

import (
	"context"

	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/joshvanl/cni-migration/pkg/types"
	"github.com/joshvanl/cni-migration/pkg/util"
)

var _ types.Step = &CleanUp{}

type CleanUp struct {
	client  *kubernetes.Clientset
	log     *logrus.Entry
	ctx     context.Context
	factory *util.Factory
}

func New(ctx context.Context, log *logrus.Entry, client *kubernetes.Clientset) types.Step {
	log = log.WithField("step", "4-cleanup")
	return &CleanUp{
		log:     log,
		client:  client,
		ctx:     ctx,
		factory: util.New(log, ctx, client),
	}
}

// Ready ensures that
// - All migration resources have been cleaned up
func (c *CleanUp) Ready() (bool, error) {
	var found bool

	for _, name := range types.DaemonSetCleanupNames {
		_, err := c.client.AppsV1().DaemonSets("kube-system").Get(c.ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			continue
		}

		if err != nil {
			return false, err
		}

		found = true
		break
	}

	c.log.Info("step 4 ready")

	return !found, nil
}

func (c *CleanUp) Run(dryrun bool) error {
	c.log.Info("Cleaning up...")

	c.log.Infof("deleting multus: %s", types.PathMultus)
	if !dryrun {
		if err := c.factory.DeleteResource(types.PathMultus, "kube-system"); err != nil {
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
