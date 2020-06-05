package util

import (
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/joshvanl/cni-migration/pkg/config"
)

func (f *Factory) Has(resources *config.Resources) (bool, error) {
	for namespace, names := range resources.DaemonSets {
		for _, name := range names {
			_, err := f.client.AppsV1().DaemonSets(namespace).Get(f.ctx, name, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			if err != nil {
				return false, err
			}
		}
	}

	for namespace, names := range resources.Deployments {
		for _, name := range names {
			_, err := f.client.AppsV1().Deployments(namespace).Get(f.ctx, name, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			if err != nil {
				return false, err
			}
		}
	}

	for namespace, names := range resources.StatefulSets {
		for _, name := range names {
			_, err := f.client.AppsV1().StatefulSets(namespace).Get(f.ctx, name, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			if err != nil {
				return false, err
			}
		}
	}

	return true, nil
}
