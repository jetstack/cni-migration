package main

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type waitFunc func() (int32, int32, error)

// waitDeploymentReadynamespace will wait for a all pods in a Deployment to become ready
func (m *migration) waitDeploymentReady(namespace, name string) error {
	return m.waitReady("Deployment", namespace, name,
		func() (int32, int32, error) {
			ds, err := m.kubeClient.AppsV1().Deployments(namespace).Get(context.TODO(), name, metav1.GetOptions{})
			if err != nil {
				return 0, 0, err
			}

			return ds.Status.ReadyReplicas, ds.Status.Replicas, nil
		})
}

// waitDaemonSetReadynamespace will wait for a all pods in a DaemonSet to become ready
func (m *migration) waitDaemonSetReady(namespace, name string) error {
	return m.waitReady("DaemonSet", namespace, name,
		func() (int32, int32, error) {
			ds, err := m.kubeClient.AppsV1().DaemonSets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
			if err != nil {
				return 0, 0, err
			}

			return ds.Status.NumberReady, ds.Status.DesiredNumberScheduled, nil
		})
}

// waitStatefulSetReadynamespace will wait for a all pods in a StatefulSet to become ready
func (m *migration) waitStatefulSetReady(namespace, name string) error {
	return m.waitReady("StatefulSet", namespace, name,
		func() (int32, int32, error) {
			ds, err := m.kubeClient.AppsV1().StatefulSets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
			if err != nil {
				return 0, 0, err
			}

			return ds.Status.ReadyReplicas, ds.Status.Replicas, nil
		})
}

func (m *migration) waitReady(kind, name, namespace string, f waitFunc) error {
	time.Sleep(time.Second / 2)

	ctx, cancel := context.WithTimeout(context.TODO(), time.Second*30)
	defer cancel()

	ticker := time.NewTicker(time.Second * 2)

	// Poll resource until it becomes updated
	for {
		ready, total, err := f()
		if err != nil {
			return err
		}

		if ready == total {
			m.log.Infof("%s %s/%s ready", kind, namespace, name)
			return nil
		}

		m.log.Infof("%s %s/%s %v/%v", kind, namespace, name, ready, total)

		select {
		case <-ctx.Done():
			return fmt.Errorf("failed to wait for %s %s/%s to become ready in time: %v/%v",
				kind, name, namespace, ready, total)
		case <-ticker.C:
		}
	}
}
