package types

import (
	"context"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
)

type Step interface {
	Ready() (bool, error)
	Run(dryrun bool) error
}

type NewFunc func(context.Context, *logrus.Entry, *kubernetes.Clientset) Step
type ReadyFunc func() (bool, error)
type RunFunc func(bool) error

const (
	LabelCanalCiliumKey   = "node-role.kubernetes.io/cilium-canal"
	LabelCanalCiliumValue = "cilium-canal"

	LabelCiliumKey   = "node-role.kubernetes.io/cilium"
	LabelCiliumValue = "cilium"

	LabelRolledKey   = "node-role.kubernetes.io/rolled"
	LabelMigratedKey = "node-role.kubernetes.io/migrated"

	PathCilium     = "cilium.yaml"
	PathMultus     = "multus-daemonset.yaml"
	PathKnetStress = "knet-stress.yaml"

	ContextSingleNodeKey = "cni-migration-single-node"
)

var (
	ResourcesDirectory = "./resources"

	DaemonSetNames = []string{
		"canal", "cilium", "cilium-migrated",
		"kube-multus", "knet-stress", "knet-stress-2",
	}

	DaemonSetCleanupNames = []string{
		"canal", "cilium", "kube-multus",
	}
)
