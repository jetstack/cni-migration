package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	CanalCiliumNodeLabel      = "node-role.kubernetes.io/cilium-canal"
	CanalCiliumNodeValueLabel = "cilium-canal"

	CiliumNodeLabel      = "node-role.kubernetes.io/cilium"
	CiliumNodeValueLabel = "cilium"

	MigratedNodeLabel = "node-role.kubernetes.io/migrated"

	CiliumYaml        = "cilium.yaml"
	MultusYaml        = "multus-daemonset.yaml"
	KnetStressYaml    = "knet-stress.yaml"
	NetworkAttachYaml = "net-attach.yaml"
)

var (
	options struct {
		resourcesDir string
	}
)

func init() {
	flag.StringVar(&options.resourcesDir, "resources-dir", "./resources", "Directory file path to resources.")
}

type migration struct {
	kubeClient *kubernetes.Clientset
	log        *logrus.Entry
}

func main() {
	flag.Parse()

	m, err := newMigration()
	if err != nil {
		logrus.Fatal(err)
	}

	if err := m.runMigration(); err != nil {
		logrus.Fatal(err)
	}

	m.log.Info("Migration was successful.")
}

func newMigration() (*migration, error) {
	kubeconfig := os.Getenv("KUBECONFIG")

	if len(kubeconfig) == 0 {
		return nil, errors.New("KUBECONFIG environment variable not set")
	}

	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build rest client: %s", err)
	}
	restConfig.Timeout = time.Second * 180

	client, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build kind kubernetes client: %s", err)
	}

	return &migration{
		kubeClient: client,
		log:        logrus.NewEntry(logrus.New()),
	}, nil
}

func (m *migration) runMigration() error {
	m.log.Infof("Running migration...")

	if err := m.ensureNodeLabels(); err != nil {
		return err
	}

	if err := m.ensureCanalNodeSelector(); err != nil {
		return err
	}

	//if err := m.ensureContolPlaneTolerations(); err != nil {
	//	return err
	//}

	if err := m.waitAllReady(); err != nil {
		return err
	}

	// Create needed resources
	if err := m.createDaemonSet(MultusYaml, "kube-system", "kube-multus-ds-amd64"); err != nil {
		return err
	}
	if err := m.createDaemonSet(CiliumYaml, "kube-system", "cilium"); err != nil {
		return err
	}
	if err := m.createDaemonSet(CiliumYaml, "kube-system", "cilium-migrated"); err != nil {
		return err
	}
	if err := m.createDaemonSet(KnetStressYaml, "knet-stress", "knet-stress"); err != nil {
		return err
	}

	// Check knet connectivity
	if err := m.checkKnetStressStatus(); err != nil {
		return err
	}

	// Apply cilium network attach
	if err := m.createResource(NetworkAttachYaml, "kube-system", "cilium-conf"); err != nil {
		return err
	}

	// Danger Zone
	// Patch all deployments etc.
	if err := m.patchAllApps(); err != nil {
		return err
	}

	// Check knet connectivity
	if err := m.checkKnetStressStatus(); err != nil {
		return err
	}

	if err := m.migrateNodes(); err != nil {
		return err
	}

	if err := m.cleanup(); err != nil {
		return err
	}

	return nil
}

func (m *migration) cleanup() error {
	m.log.Infof("Cleaning up")

	args := []string{"kubectl", "delete", "-A", "--all", "network-attachment-definitions.k8s.cni.cncf.io"}
	if err := m.runCommand(args...); err != nil {
		return err
	}

	err := m.kubeClient.AppsV1().DaemonSets("kube-system").Delete(context.TODO(), "kube-multus-ds-amd64", metav1.DeleteOptions{})
	if err != nil {
		return err
	}

	err = m.kubeClient.AppsV1().DaemonSets("kube-system").Delete(context.TODO(), "canal", metav1.DeleteOptions{})
	if err != nil {
		return err
	}

	return nil
}

func (m *migration) migrateNodes() error {
	nodes, err := m.kubeClient.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, node := range nodes.Items {

		node, err := m.kubeClient.CoreV1().Nodes().Get(context.TODO(), node.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		// Check is node needs migrating
		if val, ok := node.Labels[MigratedNodeLabel]; ok && val == "true" {
			m.log.Infof("Node already migrated %s", node.Name)
			continue
		}

		m.log.Infof("Migrating node %s", node.Name)

		m.log.Infof("Draining node %s", node.Name)
		args := []string{"kubectl", "drain", node.Name}
		if err := m.runCommand(args...); err != nil {
			return err
		}

		//args = []string{"kubectl", "taint", "node", node.Name, "node-role.kubernetes.io/cilium=cilium:NoExecute", "--overwrite"}
		//if err := m.runCommand(args...); err != nil {
		//	return err
		//}

		// TODO: remove multus config from node

		// Add taint on node
		m.log.Infof("Adding %s=%s:NoExecute taint to %s", CiliumNodeLabel, CiliumNodeValueLabel, node.Name)
		if err := m.addCiliumTaint(node.Name); err != nil {
			return err
		}

		m.log.Infof("Restarting knet-stress")
		args = []string{"kubectl", "rollout", "restart", "daemonset", "--namespace", "knet-stress", "knet-stress"}
		if err := m.runCommand(args...); err != nil {
			return err
		}

		args = []string{"kubectl", "rollout", "status", "daemonset", "--namespace", "knet-stress", "knet-stress"}
		if err := m.runCommand(args...); err != nil {
			return err
		}

		if err := m.waitDaemonSetReady("knet-stress", "knet-stress"); err != nil {
			return err
		}

		// Check knet connectivity
		if err := m.checkKnetStressStatus(); err != nil {
			return err
		}

		// Remove taint on node
		m.log.Infof("Removing %s=%s:NoExecute taint to %s", CiliumNodeLabel, CiliumNodeValueLabel, node.Name)
		if err := m.deleteCiliumTaint(node.Name); err != nil {
			return err
		}

		m.log.Infof("Uncordoning node %s", CiliumNodeLabel, CiliumNodeValueLabel, node.Name)
		args = []string{"kubectl", "uncordon", node.Name}
		if err := m.runCommand(args...); err != nil {
			return err
		}

		if err := m.waitAllReady(); err != nil {
			return err
		}

		m.log.Infof("Adding label %s=true to %s", MigratedNodeLabel, node.Name)
		if err := m.setNodeMigratedLabel(node.Name); err != nil {
			return err
		}

	}

	return nil
}

func (m *migration) createDaemonSet(yamlFileName, namespace, name string) error {
	if err := m.createResource(yamlFileName, namespace, name); err != nil {
		return err
	}

	if err := m.waitDaemonSetReady(namespace, name); err != nil {
		return err
	}

	return nil
}

func (m *migration) createResource(yamlFileName, namespace, name string) error {
	filePath := filepath.Join(options.resourcesDir, yamlFileName)

	m.log.Infof("Applying %s: %s", name, filePath)

	args := []string{"kubectl", "apply", "--namespace", namespace, "-f", filePath}
	if err := m.runCommand(args...); err != nil {
		return err
	}

	return nil
}

func (m *migration) ensureContolPlaneTolerations() error {
	for _, name := range []string{"kube-apiserver", "kube-controller-manager", "kube-scheduler"} {
		ds, err := m.kubeClient.AppsV1().DaemonSets("kube-system").Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		//var tolerations []corev1.Tol
		needsToleration := true
		for _, tol := range ds.Spec.Template.Spec.Tolerations {
			if tol.Key == CiliumNodeLabel {
				needsToleration = false
				break
			}
		}

		if needsToleration {
			m.log.Infof("Adding %s:NoExecute Toleration to %s/%s", CiliumNodeLabel, ds.Namespace, ds.Name)

			ds.Spec.Template.Spec.Tolerations = append(ds.Spec.Template.Spec.Tolerations,
				corev1.Toleration{
					Key:    CiliumNodeLabel,
					Effect: corev1.TaintEffectNoExecute,
				})

			ds, err = m.kubeClient.AppsV1().DaemonSets("kube-system").Update(context.TODO(), ds, metav1.UpdateOptions{})
			if err != nil {
				return err
			}

			if err := m.waitDaemonSetReady(ds.Namespace, ds.Name); err != nil {
				return err
			}
		}
	}

	return nil
}

// ensureCanalNodeSelector ensures that the Canal DamonSet has the correct node selector
func (m *migration) ensureCanalNodeSelector() error {
	ds, err := m.kubeClient.AppsV1().DaemonSets("kube-system").Get(context.TODO(), "canal", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get canal DaemonSet: %s", err)
	}

	// If canal DaemonSet does not have node selector than add it
	val, ok := ds.Spec.Template.Spec.NodeSelector[CanalCiliumNodeLabel]
	if !ok || val != CanalCiliumNodeValueLabel {
		m.log.Infof("Adding node selector %s=%s to DeamonSet %s/%s",
			CanalCiliumNodeLabel, CanalCiliumNodeValueLabel, ds.Namespace, ds.Name)

		if ds.Spec.Template.Spec.NodeSelector == nil {
			ds.Spec.Template.Spec.NodeSelector = make(map[string]string)
		}
		ds.Spec.Template.Spec.NodeSelector[CanalCiliumNodeLabel] = CanalCiliumNodeValueLabel

		ds, err := m.kubeClient.AppsV1().DaemonSets("kube-system").Update(context.TODO(), ds, metav1.UpdateOptions{})
		if err != nil {
			return err
		}

		if err := m.waitDaemonSetReady(ds.Namespace, ds.Name); err != nil {
			return err
		}
	}

	return nil
}

// ensureNodeLabels will initialise any node that have yet to get migration labels
func (m *migration) ensureNodeLabels() error {
	nodes, err := m.kubeClient.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, node := range nodes.Items {
		needsUpdate := false
		_, cclOK := node.Labels[CanalCiliumNodeLabel]
		_, clOK := node.Labels[CiliumNodeLabel]
		_, migOK := node.Labels[MigratedNodeLabel]

		// If we have multiple labels set then we should default to only running canal + cilium label
		if (cclOK && clOK) || (cclOK && migOK) {
			m.log.Infof("%s has multiple labels set, resetting to %s",
				node.Name, CanalCiliumNodeLabel, CiliumNodeLabel, CanalCiliumNodeLabel)

			delete(node.Labels, CiliumNodeLabel)
			delete(node.Labels, CanalCiliumNodeLabel)
			delete(node.Labels, MigratedNodeLabel)
			node.Labels[CanalCiliumNodeLabel] = CanalCiliumNodeValueLabel
			needsUpdate = true
		}

		// If neither label set then we should add canal + cilium label
		if !cclOK && !clOK && !migOK {
			m.log.Infof("%s adding label %s", node.Name, CanalCiliumNodeLabel)

			node.Labels[CanalCiliumNodeLabel] = CanalCiliumNodeValueLabel
			needsUpdate = true
		}

		if needsUpdate {
			_, err := m.kubeClient.CoreV1().Nodes().Update(context.TODO(), &node, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
		} else {
			m.log.Infof("%s already labeled", node.Name)
		}
	}

	return nil
}

func (m *migration) addCiliumTaint(nodeName string) error {
	node, err := m.kubeClient.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	hasTaint := false
	for _, t := range node.Spec.Taints {
		if t.Key == CiliumNodeLabel {
			hasTaint = true
			break
		}
	}

	if !hasTaint {
		node.Spec.Taints = append(node.Spec.Taints, corev1.Taint{
			Key:    CiliumNodeLabel,
			Value:  CiliumNodeValueLabel,
			Effect: corev1.TaintEffectNoExecute,
		})
	}

	// Change label of node
	delete(node.Labels, CanalCiliumNodeLabel)
	node.Labels[CiliumNodeLabel] = CiliumNodeValueLabel

	node, err = m.kubeClient.CoreV1().Nodes().Update(context.TODO(), node, metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	return nil
}

func (m *migration) deleteCiliumTaint(nodeName string) error {
	node, err := m.kubeClient.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	var taints []corev1.Taint
	for _, t := range node.Spec.Taints {
		if t.Key != CiliumNodeLabel {
			taints = append(taints, t)
		}
	}
	node.Spec.Taints = taints

	_, err = m.kubeClient.CoreV1().Nodes().Update(context.TODO(), node, metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	return nil
}

func (m *migration) setNodeMigratedLabel(nodeName string) error {
	node, err := m.kubeClient.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	// Set migrated label
	//delete(node.Labels, CiliumNodeLabel)
	delete(node.Labels, CanalCiliumNodeLabel)
	node.Labels[MigratedNodeLabel] = "true"

	_, err = m.kubeClient.CoreV1().Nodes().Update(context.TODO(), node, metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	return nil
}
