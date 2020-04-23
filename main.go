package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	CanalCiliumNodeLabel      = "node.role/node-role.kubernetes.io/cilium-canal"
	CanalCiliumNodeValueLabel = "cilium-canal"

	CiliumNodeLabel      = "node.role/node-role.kubernetes.io/cilium"
	CiliumNodeValueLabel = "cilium"

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
	if err := m.ensureNodeLabels(); err != nil {
		return err
	}

	if err := m.ensureCanalNodeSelector(); err != nil {
		return err
	}

	// Create needed resources
	if err := m.createDaemonSet(CiliumYaml, "kube-system", "cilium"); err != nil {
		return err
	}
	if err := m.createDaemonSet(MultusYaml, "kube-system", "multus"); err != nil {
		return err
	}
	if err := m.createDaemonSet(KnetStressYaml, "knet-stress", "knet-stress"); err != nil {
		return err
	}

	// Check knet connectivity
	if err := m.checkKnetStressStatus(); err != nil {
		return err
	}

	// Apply cilium network attach in all namespaces
	nss, err := m.kubeClient.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, ns := range nss.Items {
		if err := m.createResource(NetworkAttachYaml, ns.Name, "cilium-conf"); err != nil {
			return err
		}
	}

	// Danger Zone
	// Patch all deployments etc.
	if err := m.patchAllApps(nss); err != nil {
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
	m.log.Infof("%s", args)
	err := exec.Command(args[0], args[1:]...).Run()
	if err != nil {
		return err
	}

	err = m.kubeClient.AppsV1().DaemonSets("kube-system").Delete(context.TODO(), "multus", metav1.DeleteOptions{})
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
		m.log.Infof("Migrating node %s", node.Name)

		args := []string{"kubectl", "drain", node.Name}
		m.log.Infof("%s", args)
		err := exec.Command(args[0], args[1:]...).Run()
		if err != nil {
			return err
		}

		// TODO: remove multus config from node

		node, err := m.kubeClient.CoreV1().Nodes().Get(context.TODO(), node.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		// Change label of node
		delete(node.Labels, CanalCiliumNodeLabel)
		node.Labels[CiliumNodeLabel] = CiliumNodeValueLabel

		_, err = m.kubeClient.CoreV1().Nodes().Update(context.TODO(), node, metav1.UpdateOptions{})
		if err != nil {
			return err
		}

		// TODO: kill all pods on node

		args = []string{"kubectl", "uncordon", node.Name}
		m.log.Infof("%s", args)
		err = exec.Command(args[0], args[1:]...).Run()
		if err != nil {
			return err
		}

		// TODO: wait all ready

		// Check knet connectivity
		if err := m.checkKnetStressStatus(); err != nil {
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

	m.log.Infof("Applying %s: %s/%s", name, filePath)

	args := []string{"kubectl", "apply", "--namespace", namespace, "-f", filePath}
	m.log.Infof("%s", args)
	err := exec.Command(args[0], args[1:]...).Run()
	if err != nil {
		return err
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

		// If we have both labels set then we should default to only running canal + cilium label
		if cclOK && clOK {
			m.log.Infof("%s has both %s and %s labels set, resetting to %s",
				node.Name, CanalCiliumNodeLabel, CiliumNodeLabel, CanalCiliumNodeLabel)
			delete(node.Labels, CiliumNodeLabel)
			needsUpdate = true
		}

		// If neither label set then we should add canal + cilium label
		if !cclOK && !clOK {
			m.log.Infof("%s adding label %s", node.Name, CiliumNodeLabel)

			node.Labels[CiliumNodeLabel] = CiliumNodeValueLabel
			needsUpdate = true
		}

		if needsUpdate {
			_, err := m.kubeClient.CoreV1().Nodes().Update(context.TODO(), &node, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
		}
	}

	return nil
}
