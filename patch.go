package main

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	NetAttachAnnotationKey   = "k8s.v1.cni.cncf.io/networks"
	NetAttachAnnotationValue = "kube-system/cilium-conf"
)

func (m *migration) patchAllApps() error {
	nss, err := m.kubeClient.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

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
			if deploy.Spec.Template.Spec.HostNetwork {
				m.log.Infof("Skipping Deployment with host networking %s/%s", deploy.Namespace, deploy.Name)
				continue
			}

			if val, ok := deploy.Spec.Template.Annotations[NetAttachAnnotationKey]; !ok || val != NetAttachAnnotationValue {
				m.log.Infof("Adding network-attachment to Deployment %s/%s %s=%s",
					deploy.Namespace, deploy.Name, NetAttachAnnotationKey, NetAttachAnnotationValue)

				if deploy.Spec.Template.Annotations == nil {
					deploy.Spec.Template.Annotations = make(map[string]string)
				}
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
			if ds.Spec.Template.Spec.HostNetwork {
				m.log.Infof("Skipping DaemonSet with host networking %s/%s", ds.Namespace, ds.Name)
				continue
			}

			if val, ok := ds.Spec.Template.Annotations[NetAttachAnnotationKey]; !ok || val != NetAttachAnnotationValue {
				m.log.Infof("Adding network-attachment to DaemonSet %s/%s %s=%s",
					ds.Namespace, ds.Name, NetAttachAnnotationKey, NetAttachAnnotationValue)

				if ds.Spec.Template.Annotations == nil {
					ds.Spec.Template.Annotations = make(map[string]string)
				}
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
			if ss.Spec.Template.Spec.HostNetwork {
				m.log.Infof("Skipping StatefulSet with host networking %s/%s", ss.Namespace, ss.Name)
				continue
			}

			if val, ok := ss.Spec.Template.Annotations[NetAttachAnnotationKey]; !ok || val != NetAttachAnnotationValue {
				m.log.Infof("Adding network-attachment to StatefulSet %s/%s %s=%s",
					ss.Namespace, ss.Name, NetAttachAnnotationKey, NetAttachAnnotationValue)

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
