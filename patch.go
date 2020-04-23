package main

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	NetAttachAnnotationKey   = "k8s.v1.cni.cncf.io/networks"
	NetAttachAnnotationValue = "cilium-conf"
)

func (m *migration) patchAllApps(nss *corev1.NamespaceList) error {
	if err := m.patchDeloyments(nss); err != nil {
		return err
	}

	if err := m.patchDaemonSets(nss); err != nil {
		return err
	}

	if err := m.patchStatefulSets(nss); err != nil {
		return err
	}

	return nil
}

func (m *migration) patchDeloyments(nss *corev1.NamespaceList) error {
	for _, ns := range nss.Items {
		deploys, err := m.kubeClient.AppsV1().Deployments(ns.Name).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return err
		}

		// If does not have network attachment annotation, add and apply
		for _, deploy := range deploys.Items {
			if val, ok := deploy.Spec.Template.Annotations[NetAttachAnnotationKey]; !ok || val != NetAttachAnnotationValue {
				m.log.Infof("Adding network-attachment to Deployment %s=%s", NetAttachAnnotationKey, NetAttachAnnotationValue)

				deploy.Spec.Template.Annotations[NetAttachAnnotationKey] = NetAttachAnnotationValue

				_, err := m.kubeClient.AppsV1().Deployments(deploy.Namespace).Update(context.TODO(), &deploy, metav1.UpdateOptions{})
				if err != nil {
					return err
				}

				if err := m.waitDeploymentReady(deploy.Namespace, deploy.Name); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (m *migration) patchDaemonSets(nss *corev1.NamespaceList) error {
	for _, ns := range nss.Items {
		ds, err := m.kubeClient.AppsV1().DaemonSets(ns.Name).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return err
		}

		// If does not have network attachment annotation, add and apply
		for _, ds := range ds.Items {
			if val, ok := ds.Spec.Template.Annotations[NetAttachAnnotationKey]; !ok || val != NetAttachAnnotationValue {
				m.log.Infof("Adding network-attachment to DaemonSet %s=%s", NetAttachAnnotationKey, NetAttachAnnotationValue)

				ds.Spec.Template.Annotations[NetAttachAnnotationKey] = NetAttachAnnotationValue

				_, err := m.kubeClient.AppsV1().DaemonSets(ds.Namespace).Update(context.TODO(), &ds, metav1.UpdateOptions{})
				if err != nil {
					return err
				}

				if err := m.waitDaemonSetReady(ds.Namespace, ds.Name); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (m *migration) patchStatefulSets(nss *corev1.NamespaceList) error {
	for _, ns := range nss.Items {
		ss, err := m.kubeClient.AppsV1().StatefulSets(ns.Name).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return err
		}

		// If does not have network attachment annotation, add and apply
		for _, ss := range ss.Items {
			if val, ok := ss.Spec.Template.Annotations[NetAttachAnnotationKey]; !ok || val != NetAttachAnnotationValue {
				m.log.Infof("Adding network-attachment to StatefulSet %s=%s", NetAttachAnnotationKey, NetAttachAnnotationValue)

				ss.Spec.Template.Annotations[NetAttachAnnotationKey] = NetAttachAnnotationValue

				_, err := m.kubeClient.AppsV1().StatefulSets(ss.Namespace).Update(context.TODO(), &ss, metav1.UpdateOptions{})
				if err != nil {
					return err
				}

				if err := m.waitStatefulSetReady(ss.Namespace, ss.Name); err != nil {
					return err
				}
			}
		}
	}

	return nil
}
