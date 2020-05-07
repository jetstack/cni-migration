package runner

import (
	"fmt"
)

type Runner struct {
	log    *logrus.Entry
	client *kubernetes.Client
}

func New(log *logrus.Entry, mclient *kubernetes.Clientset) *Runner {
	return &Runner{
		client: client,
		log:    log,
	}
}

// Set 1
// InitCluster will ensure node labels, install the required resources, and
// patch canal.
func (r *Runner) InitCluster() error {
}
