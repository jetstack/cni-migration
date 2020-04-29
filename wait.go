package main

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type waitFunc func() (int32, int32, error)

func (m *migration) waitAllReady() error {

	nss, err := m.kubeClient.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, ns := range nss.Items {
		deploys, err := m.kubeClient.AppsV1().Deployments(ns.Name).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return err
		}

		for _, deploy := range deploys.Items {
			if err := m.waitDeploymentReady(ns.Name, deploy.Name); err != nil {
				return err
			}
		}

		dss, err := m.kubeClient.AppsV1().DaemonSets(ns.Name).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return err
		}

		for _, ds := range dss.Items {
			if err := m.waitDaemonSetReady(ns.Name, ds.Name); err != nil {
				return err
			}
		}

		sss, err := m.kubeClient.AppsV1().StatefulSets(ns.Name).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return err
		}

		for _, ss := range sss.Items {
			if err := m.waitStatefulSetReady(ns.Name, ss.Name); err != nil {
				return err
			}
		}
	}

	return nil
}

// waitDeploymentReadynamespace will wait for a all pods in a Deployment to become ready
func (m *migration) waitDeploymentReady(namespace, name string) error {
	return m.waitReady("deployment", name, namespace)
}

// waitDaemonSetReadynamespace will wait for a all pods in a DaemonSet to become ready
func (m *migration) waitDaemonSetReady(namespace, name string) error {
	return m.waitReady("daemonset", name, namespace)
}

// waitStatefulSetReadynamespace will wait for a all pods in a StatefulSet to become ready
func (m *migration) waitStatefulSetReady(namespace, name string) error {
	return m.waitReady("statefulset", name, namespace)
}

func (m *migration) waitReady(kind, name, namespace string) error {
	args := []string{"kubectl", "rollout", "status", kind, "--namespace", namespace, name}
	if err := m.runCommand(args...); err != nil {
		return err
	}
	return nil
}
