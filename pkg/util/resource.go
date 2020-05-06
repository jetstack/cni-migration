package util

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"

	"github.com/joshvanl/cni-migration/pkg/types"
)

type Factory struct {
	log    *logrus.Entry
	ctx    context.Context
	client *kubernetes.Clientset
}

func New(log *logrus.Entry, ctx context.Context, client *kubernetes.Clientset) *Factory {
	return &Factory{
		log:    log,
		ctx:    ctx,
		client: client,
	}
}

func (f *Factory) CreateDaemonSet(yamlFilePath, namespace, name string) error {
	if err := f.createResource(yamlFilePath, namespace, name); err != nil {
		return err
	}

	if err := f.waitDaemonSetReady(namespace, name); err != nil {
		return err
	}

	return nil
}

func (f *Factory) createResource(yamlFilePath, namespace, name string) error {
	filePath := filepath.Join(types.ResourcesDirectory, yamlFilePath)

	f.log.Debugf("Applying %s: %s", name, filePath)

	args := []string{"kubectl", "apply", "--namespace", namespace, "-f", filePath}
	if err := f.runCommand(args...); err != nil {
		return err
	}

	return nil
}

func (f *Factory) runCommand(args ...string) error {
	f.log.Debugf("%s", args)

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = nil
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return err
	}

	return nil
}
