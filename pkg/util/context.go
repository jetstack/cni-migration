package util

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func NodesFromContext(client *kubernetes.Clientset, ctx context.Context, key string) ([]corev1.Node, bool, error) {
	v := ctx.Value(key)
	if v == nil {
		return nil, false, nil
	}

	nodeNames, ok := v.([]string)
	if !ok {
		return nil, false, fmt.Errorf("failed to get node names from context: %#+v", v)
	}

	var nodes []corev1.Node
	for _, nodeName := range nodeNames {
		node, err := client.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
		if err != nil {
			return nil, false, fmt.Errorf("failed to find node %s: %s", nodeName, err)
		}

		nodes = append(nodes, *node)
	}

	return nodes, true, nil
}
