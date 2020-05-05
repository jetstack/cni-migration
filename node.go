package main

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (m *migration) rollNode(nodeName string) error {
	node, err := m.kubeClient.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	if val, ok := node.Labels[RolledNodeLabel]; ok && val == "true" {
		return nil
	}

	m.log.Infof("%s Draining node", nodeName)
	args := []string{"kubectl", "drain", "--delete-local-data", "--ignore-daemonsets", nodeName}
	if err := m.runCommand(args...); err != nil {
		return err
	}

	if err := m.waitAllReady(); err != nil {
		return err
	}

	// Delete all pods on that node
	m.log.Infof("%s Deleting all pods on node", nodeName)
	if err := m.deletePodsOnNode(nodeName); err != nil {
		return err
	}

	m.log.Infof("%s Uncordoning node", nodeName)
	args = []string{"kubectl", "uncordon", nodeName}
	if err := m.runCommand(args...); err != nil {
		return err
	}

	if err := m.waitAllReady(); err != nil {
		return err
	}

	if err := m.checkKnetStressStatus(); err != nil {
		return err
	}

	node, err = m.kubeClient.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	if node.Labels == nil {
		node.Labels = make(map[string]string)
	}
	node.Labels[RolledNodeLabel] = "true"

	_, err = m.kubeClient.CoreV1().Nodes().Update(context.TODO(), node, metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	return nil
}
