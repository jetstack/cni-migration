package util

import (
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (f *Factory) CheckKnetStress() error {
	f.log.Info("checking knet-stress connectivity...")

	if err := f.WaitDaemonSetReady("knet-stress", "knet-stress"); err != nil {
		return err
	}

	ticker := time.NewTicker(time.Second * 5)

	for {
		pods, err := f.client.CoreV1().Pods("knet-stress").List(f.ctx, metav1.ListOptions{
			LabelSelector: "app=knet-stress",
		})
		if err != nil {
			return err
		}

		ready := true
		for _, pod := range pods.Items {
			args := []string{"kubectl", "exec", "--namespace", "knet-stress", pod.Name, "--", "/knet-stress", "-status"}
			if err := f.RunCommand(args...); err != nil {
				f.log.Error(err.Error())
				ready = false
				break
			}
		}

		if ready {
			return nil
		}

		select {
		case <-f.ctx.Done():
			return fmt.Errorf("knet-stress connectivity failed: %s", f.ctx.Err())
		case <-ticker.C:
			continue
		}
	}
}
