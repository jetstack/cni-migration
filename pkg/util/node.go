package util

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/joshvanl/cni-migration/pkg/config"
)

func (f *Factory) RollNode(dryrun bool, nodeName string, watchResources *config.Resources) error {
	f.log.Infof("draining node %s", nodeName)

	if !dryrun {
		args := []string{"kubectl", "drain", "--delete-local-data", "--ignore-daemonsets", nodeName}
		if err := f.RunCommand(nil, args...); err != nil {
			return err
		}

		if err := f.WaitAllReady(watchResources); err != nil {
			return err
		}
	}

	// Delete all pods on that node
	f.log.Infof("deleting all pods on node %s", nodeName)
	if !dryrun {
		if err := f.DeletePodsOnNode(nodeName); err != nil {
			return err
		}
	}

	f.log.Infof("uncordoning node %s", nodeName)
	if !dryrun {
		args := []string{"kubectl", "uncordon", nodeName}
		if err := f.RunCommand(nil, args...); err != nil {
			return err
		}

		if err := f.WaitAllReady(watchResources); err != nil {
			return err
		}

		if err := f.CheckKnetStress(); err != nil {
			return err
		}
	}

	return nil
}

func (f *Factory) DeletePodsOnNode(nodeName string) error {
	nss, err := f.client.CoreV1().Namespaces().List(f.ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}

	toBeDeleted := make(map[*corev1.Pod]struct{})

	for _, ns := range nss.Items {
		pods, err := f.client.CoreV1().Pods(ns.Name).List(f.ctx, metav1.ListOptions{})
		if err != nil {
			return err
		}

		for _, p := range pods.Items {
			if p.Spec.NodeName == nodeName && !p.Spec.HostNetwork {
				f.log.Debugf("deleting pod %s on node %s", p.Name, nodeName)
				toBeDeleted[p.DeepCopy()] = struct{}{}

				err = f.client.CoreV1().Pods(ns.Name).Delete(f.ctx, p.Name, metav1.DeleteOptions{})
				if err != nil {
					return err
				}
			}
		}
	}

	// Wait for pods to be deleted
	for {
		for p := range toBeDeleted {
			_, err = f.client.CoreV1().Pods(p.Namespace).Get(f.ctx, p.Name, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				delete(toBeDeleted, p)
				continue
			}
			if err != nil {
				return err
			}
		}

		if len(toBeDeleted) == 0 {
			break
		}

		time.Sleep(time.Second)
	}

	return nil
}
