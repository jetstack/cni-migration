package util

import (
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/jetstack/cni-migration/pkg/config"
)

func (f *Factory) Delete(resources *config.Resources) error {
	for namespace, names := range resources.DaemonSets {
		for _, name := range names {
			err := f.client.AppsV1().DaemonSets(namespace).Delete(f.ctx, name, metav1.DeleteOptions{})
			if apierrors.IsNotFound(err) {
				continue
			}
			if err != nil {
				return err
			}
		}
	}

	for namespace, names := range resources.Deployments {
		for _, name := range names {
			err := f.client.AppsV1().Deployments(namespace).Delete(f.ctx, name, metav1.DeleteOptions{})
			if apierrors.IsNotFound(err) {
				continue
			}
			if err != nil {
				return err
			}
		}
	}

	for namespace, names := range resources.StatefulSets {
		for _, name := range names {
			err := f.client.AppsV1().StatefulSets(namespace).Delete(f.ctx, name, metav1.DeleteOptions{})
			if apierrors.IsNotFound(err) {
				continue
			}
			if err != nil {
				return err
			}
		}
	}

	return nil
}
