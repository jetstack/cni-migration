package app

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/klog"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

func (o *Options) AddFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&o.NoDryRun, "no-dry-run", false, "Run the CLI tool _not_ in dry run mode. This will attempt to migrate your cluster.")
	fs.BoolVar(&o.StepAll, "step-all", false, "Run all steps. Cannot be used in conjunction with other step options.")
	fs.BoolVarP(&o.StepPreflight, "step-preflight", "0", false, "[0] - Install knet-stress and ensure connectivity.")
	fs.BoolVarP(&o.StepPrepare, "step-prepare", "1", false, "[1] - Install required resource and prepare cluster.")
	fs.BoolVarP(&o.StepRollNodes, "step-roll-nodes", "2", false, "[2] - Roll all nodes on the cluster to install both CNIs to workloads.")
	fs.BoolVarP(&o.StepChangeCNIPriority, "step-change-cni-priority", "3", false, "[3] - Change the CNI priority to Cilium.")
	fs.StringVar(&o.StepMigrateNode, "step-migrate-node", "", "[4] - Migrate a single node in the cluster by node name.")
	fs.BoolVarP(&o.StepMigrateAllNodes, "step-migrate-all-nodes", "4", false, "[4] - Migrate all nodes in the cluster, one by one.")
	fs.BoolVarP(&o.StepCleanUp, "step-clean-up", "5", false, "[5] - Clean up migration resources.")
	fs.StringVarP(&o.LogLevel, "log-level", "v", "debug", "Set logging level [debug|info|warn|error|fatal]")
	fs.StringVarP(&o.ConfigPath, "config", "c", "config.yaml", "File path to the config path.")
}

func AddKubeFlags(cmd *cobra.Command, fs *pflag.FlagSet) cmdutil.Factory {
	kubeConfigFlags := genericclioptions.NewConfigFlags(true)
	kubeConfigFlags.AddFlags(fs)
	matchVersionKubeConfigFlags := cmdutil.NewMatchVersionFlags(kubeConfigFlags)
	matchVersionKubeConfigFlags.AddFlags(fs)
	factory := cmdutil.NewFactory(matchVersionKubeConfigFlags)

	cmd.Flags().AddGoFlagSet(flag.CommandLine)
	flag.CommandLine.Parse([]string{})
	fakefs := flag.NewFlagSet("fake", flag.ExitOnError)
	klog.InitFlags(fakefs)
	if err := fakefs.Parse([]string{"-logtostderr=false"}); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}

	return factory
}

func (o *Options) Validate() error {
	if o.StepMigrateAllNodes && len(o.StepMigrateNode) > 0 {
		return errors.New("cannot enable both --step-migrate-all-nodes, as well as --step-migrate-node")
	}

	if o.StepAll {
		switch o.StepAll {
		case o.StepPreflight, o.StepPrepare, o.StepRollNodes, len(o.StepMigrateNode) > 0, o.StepMigrateAllNodes, o.StepCleanUp:
			return errors.New("no other step flags may be enabled with --step-all")
		}
	}

	return nil
}
