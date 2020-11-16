package util

import (
	"github.com/jetstack/cni-migration/pkg/config"
)

func (f *Factory) WaitAllReady(resources *config.Resources) error {
	for namespace, names := range resources.Deployments {
		for _, name := range names {
			if err := f.waitDeploymentReady(namespace, name); err != nil {
				return err
			}
		}
	}

	for namespace, names := range resources.DaemonSets {
		for _, name := range names {
			if err := f.WaitDaemonSetReady(namespace, name); err != nil {
				return err
			}
		}
	}

	for namespace, names := range resources.StatefulSets {
		for _, name := range names {
			if err := f.waitStatefulSetReady(namespace, name); err != nil {
				return err
			}
		}
	}

	return nil
}

// waitDeploymentReadynamespace will wait for a all pods in a Deployment to become ready
func (f *Factory) waitDeploymentReady(namespace, name string) error {
	return f.waitReady("deployment", name, namespace)
}

// waitDaemonSetReadynamespace will wait for a all pods in a DaemonSet to become ready
func (f *Factory) WaitDaemonSetReady(namespace, name string) error {
	return f.waitReady("daemonset", name, namespace)
}

// waitStatefulSetReadynamespace will wait for a all pods in a StatefulSet to become ready
func (f *Factory) waitStatefulSetReady(namespace, name string) error {
	return f.waitReady("statefulset", name, namespace)
}

func (f *Factory) waitReady(kind, name, namespace string) error {
	args := []string{"kubectl", "rollout", "status", kind, "--namespace", namespace, name}
	if err := f.RunCommand(nil, args...); err != nil {
		return err
	}
	return nil
}
