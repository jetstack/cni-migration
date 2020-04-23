package main

import (
	"context"
	"errors"
	"os/exec"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (m *migration) checkKnetStressStatus() error {
	m.log.Info("Checking knet-stress connectivity")

	if err := m.waitDaemonSetReady("knet-stress", "knet-stress"); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.TODO(), time.Second*30)
	defer cancel()

	ticker := time.NewTicker(time.Second * 2)

	for {
		pods, err := m.kubeClient.CoreV1().Pods("knet-stress").List(context.TODO(), metav1.ListOptions{
			LabelSelector: "app=knet-stress",
		})
		if err != nil {
			return err
		}

		ready := true
		for _, pod := range pods.Items {
			args := []string{"kubectl", "exec", "--namespace", "knet-stress", "-it", pod.Name, "--status"}
			m.log.Infof("%s", args)

			err := exec.Command(args[0], args[1:]...).Run()
			if err != nil {
				m.log.Error(err.Error())
				ready = false
				break
			}
		}

		if ready {
			return nil
		}

		select {
		case <-ctx.Done():
			return errors.New("knet-stress connectivity failed")
		case <-ticker.C:
			continue
		}
	}
}
