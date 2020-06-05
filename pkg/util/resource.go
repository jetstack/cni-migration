package util

import (
	"context"
	"os"
	"os/exec"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
)

type Factory struct {
	ctx context.Context

	log    *logrus.Entry
	client *kubernetes.Clientset
}

func New(ctx context.Context, log *logrus.Entry, client *kubernetes.Clientset) *Factory {
	return &Factory{
		ctx:    ctx,
		log:    log,
		client: client,
	}
}

func (f *Factory) CreateDaemonSet(filePath, namespace, name string) error {
	if err := f.createResource(filePath, namespace, name); err != nil {
		return err
	}

	if err := f.WaitDaemonSetReady(namespace, name); err != nil {
		return err
	}

	return nil
}

func (f *Factory) createResource(filePath, namespace, name string) error {
	f.log.Debugf("applying %s: %s", name, filePath)

	args := []string{"kubectl", "apply", "--namespace", namespace, "-f", filePath}
	if err := f.RunCommand(args...); err != nil {
		return err
	}

	return nil
}

func (f *Factory) DeleteResource(filePath, namespace string) error {
	f.log.Debugf("deleting %s", filePath)

	args := []string{"kubectl", "delete", "--namespace", namespace, "-f", filePath}
	if err := f.RunCommand(args...); err != nil {
		return err
	}

	return nil
}

func (f *Factory) RunCommand(args ...string) error {
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
