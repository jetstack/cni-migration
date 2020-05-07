package util

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (f *Factory) WaitAllReady() error {
	nss, err := f.client.CoreV1().Namespaces().List(f.ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, ns := range nss.Items {
		deploys, err := f.client.AppsV1().Deployments(ns.Name).List(f.ctx, metav1.ListOptions{})
		if err != nil {
			return err
		}

		for _, deploy := range deploys.Items {
			if err := f.waitDeploymentReady(ns.Name, deploy.Name); err != nil {
				return err
			}
		}

		dss, err := f.client.AppsV1().DaemonSets(ns.Name).List(f.ctx, metav1.ListOptions{})
		if err != nil {
			return err
		}

		for _, ds := range dss.Items {
			if err := f.WaitDaemonSetReady(ns.Name, ds.Name); err != nil {
				return err
			}
		}

		sss, err := f.client.AppsV1().StatefulSets(ns.Name).List(f.ctx, metav1.ListOptions{})
		if err != nil {
			return err
		}

		for _, ss := range sss.Items {
			if err := f.waitStatefulSetReady(ns.Name, ss.Name); err != nil {
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
	if err := f.RunCommand(args...); err != nil {
		return err
	}
	return nil
}
