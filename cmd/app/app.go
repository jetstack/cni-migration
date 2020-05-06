package app

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	// Load all auth plugins
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	cliflag "k8s.io/component-base/cli/flag"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

type Options struct {
	NoDryRun bool
	LogLevel string

	StepAll               bool
	StepInstallCNIs       bool
	StepRollNodes         bool
	StepMigrateSingleNode bool
	StepMigrateAllNodes   bool
	StepCleanUp           bool
}

const (
	long = `  cni-migration is a CLI tool to migrate a Kubernetes cluster from using Calico
  to Cilium. By default, the CLI tool will run in debug mode, and not perform any
  steps. All previous steps must be successful in order to run further steps.`
	examples = `
  # Execute a dry run of a full migration
  cni-migration --all

  # Perform a migration only the first 2 steps
  cni-migration --no-dry-run -1 -2

  # Perform a full live migration
  cni-migration --no-dry-run --all`
)

func NewRunCmd(ctx context.Context) *cobra.Command {
	var factory cmdutil.Factory

	o := new(Options)

	cmd := &cobra.Command{
		Use:     "cni-migration",
		Short:   "cni-migration is a CLI tool to migrate a Kubernetes cluster from using Calico to Cilium.",
		Long:    long,
		Example: examples,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := o.Validate(); err != nil {
				return err
			}

			lvl, err := logrus.ParseLevel(o.LogLevel)
			if err != nil {
				return fmt.Errorf("failed to parse --log-level: %s", err)
			}

			logger := logrus.New()
			logger.SetLevel(lvl)
			log := logrus.NewEntry(logger)
			// set log  value to dry run if set

			if o.StepMigrateSingleNode {
				ctx.Value(type.ContextSingleNodeKey) = "true"
			}

			client, err := factory.KubernetesClientSet()
			if err != nil {
				return fmt.Errorf("failed to build kubernetes client: %s", err)
			}

			return nil
		},
	}

	nfs := new(cliflag.NamedFlagSets)

	// pretty output from kube-apiserver
	usageFmt := "Usage:\n  %s\n\n"
	cmd.SetUsageFunc(func(cmd *cobra.Command) error {
		fmt.Fprintf(cmd.OutOrStderr(), usageFmt, cmd.UseLine())
		cliflag.PrintSections(cmd.OutOrStderr(), *nfs, -1)
		return nil
	})

	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		fmt.Fprintf(cmd.OutOrStdout(), "%s\n\n"+usageFmt, cmd.Long, cmd.UseLine())
		fmt.Fprintf(cmd.OutOrStdout(), "Examples:%s\n", cmd.Example)
		cliflag.PrintSections(cmd.OutOrStdout(), *nfs, -1)
	})

	o.AddFlags(nfs.FlagSet("Option"))
	factory = AddKubeFlags(cmd, nfs.FlagSet("Client"))

	fs := cmd.Flags()
	for n, f := range nfs.FlagSets {
		fmt.Printf("%s\n", n)
		fs.AddFlagSet(f)
	}

	return cmd
}
