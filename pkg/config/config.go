package config

import (
	"fmt"
	"io/ioutil"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	"k8s.io/client-go/kubernetes"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

type Labels struct {
	CanalCilium       string `yaml:"canal-cilium"`
	Rolled            string `yaml:"rolled"`
	CNIPriorityCanal  string `yaml:"cni-priority-canal"`
	CNIPriorityCilium string `yaml:"cni-priority-cilium"`

	Cilium   string `yaml:"cilium"`
	Migrated string `yaml:"migrated"`

	Value string `yaml:"value"`
}

type Paths struct {
	KnetStress string `yaml:"knet-stress"`
	Cilium     string `yaml:"cilium"`
	Multus     string `yaml:"multus"`
}

type Resources struct {
	DaemonSets   map[string][]string `yaml:"daemonsets"`
	Deployments  map[string][]string `yaml:"deployments"`
	StatefulSets map[string][]string `yaml:"statefulsets"`
}

type Config struct {
	*Labels            `yaml:"labels"`
	*Paths             `yaml:"paths"`
	PreflightResources *Resources `yaml:"preflightResources"`
	WatchedResources   *Resources `yaml:"watchedResources"`
	CleanUpResources   *Resources `yaml:"cleanUpResources"`

	Client *kubernetes.Clientset
	Log    *logrus.Entry
}

func New(configPath string, logLevel logrus.Level, kubeFactory cmdutil.Factory) (*Config, error) {
	yamlFile, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config path %q: %s",
			configPath, err)
	}

	config := new(Config)
	if err := yaml.UnmarshalStrict(yamlFile, config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config %q: %s",
			configPath, err)
	}

	config.Client, err = kubeFactory.KubernetesClientSet()
	if err != nil {
		return nil, fmt.Errorf("failed to build kubernetes client: %s", err)
	}

	logger := logrus.New()
	logger.SetLevel(logLevel)
	config.Log = logrus.NewEntry(logger)

	return config, nil
}
