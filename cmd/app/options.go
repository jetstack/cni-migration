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

	fs.BoolVarP(&o.StepRollAllNodes, "step-roll-all-nodes", "2", false, "[2] - Roll all nodes on the cluster to install both CNIs to workloads.")
	fs.StringSliceVar(&o.StepRollNodes, "step-roll-nodes", nil, "[2] - Roll a list of nodes on the cluster to install both CNIs to workloads by node name.")

	fs.BoolVarP(&o.StepChangeCNIAllPriority, "step-change-cni-all-priority", "3", false, "[3] - Change the CNI priority to Cilium on all nodes.")
	fs.StringSliceVar(&o.StepChangeCNIPriority, "step-change-cni-priority", nil, "[3] - Change the CNI priority to Cilium of a list of nodes by node name.")

	fs.StringSliceVar(&o.StepMigrateNodes, "step-migrate-nodes", nil, "[4] - Migrate a list of nodes in the cluster by node name.")
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
	if o.StepMigrateAllNodes && len(o.StepMigrateNodes) > 0 {
		return errors.New("cannot enable both --step-migrate-all-nodes, as well as --step-migrate-nodes")
	}

	if o.StepRollAllNodes && len(o.StepRollNodes) > 0 {
		return errors.New("cannot enable both --step-roll-all-nodes, as well as --step-roll-nodes")
	}

	if o.StepChangeCNIAllPriority && len(o.StepChangeCNIPriority) > 0 {
		return errors.New("cannot enable both --step-change-all-cni-priority, as well as --step-change-cni-priority")
	}

	if o.StepAll {
		switch o.StepAll {
		case o.StepPreflight, o.StepPrepare, o.StepRollAllNodes,
			len(o.StepMigrateNodes) > 0, o.StepMigrateAllNodes, o.StepCleanUp:

			return errors.New("no other step flags may be enabled with --step-all")
		}
	}

	return nil
}
