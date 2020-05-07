package app

import (
	"context"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"

	// Load all auth plugins
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	cliflag "k8s.io/component-base/cli/flag"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	"github.com/joshvanl/cni-migration/pkg/cleanup"
	"github.com/joshvanl/cni-migration/pkg/migrate"
	"github.com/joshvanl/cni-migration/pkg/prepare"
	"github.com/joshvanl/cni-migration/pkg/roll"
	"github.com/joshvanl/cni-migration/pkg/types"
)

type Options struct {
	NoDryRun bool
	LogLevel string

	StepAll               bool
	StepPrepare           bool
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
  cni-migration --step-all

  # Perform a migration only the first 2 steps
  cni-migration --no-dry-run -1 -2

  # Perform a full live migration
  cni-migration --no-dry-run --step-all`
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
				ctx = context.WithValue(ctx, types.ContextSingleNodeKey, "true")
			}

			client, err := factory.KubernetesClientSet()
			if err != nil {
				return fmt.Errorf("failed to build kubernetes client: %s", err)
			}

			if err := run(ctx, log, client, o); err != nil {
				log.Error(err)
				os.Exit(1)
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
	for _, f := range nfs.FlagSets {
		fs.AddFlagSet(f)
	}

	return cmd
}

func run(ctx context.Context, log *logrus.Entry, client *kubernetes.Clientset, o *Options) error {
	dryrun := !o.NoDryRun

	if dryrun {
		log = log.WithField("dry-run", "true")
	}

	var steps []types.Step
	for _, f := range []types.NewFunc{
		prepare.New,
		roll.New,
		migrate.New,
		cleanup.New,
	} {
		steps = append(steps, f(ctx, log, client))
	}

	if o.StepAll {
		for _, s := range steps {
			if err := s.Run(dryrun); err != nil {
				return err
			}
		}

		return nil
	}

	stepBool := []bool{
		o.StepPrepare,
		o.StepRollNodes,
		(o.StepMigrateSingleNode || o.StepMigrateAllNodes),
		o.StepCleanUp,
	}

	maxStep := -1
	for i, b := range stepBool {
		if b {
			maxStep = i
		}
	}

	if maxStep == -1 {
		log.Info("no steps specified")
	}

	for i, enabled := range stepBool {
		if i > maxStep {
			break
		}

		if enabled {
			if err := steps[i].Run(dryrun); err != nil {
				return err
			}
		} else {
			ready, err := steps[i].Ready()
			if err != nil {
				return fmt.Errorf("step %d failed: %s", i+1, err)
			}

			if !ready {
				return fmt.Errorf("step %d not ready...", i+1)
			}
		}
	}

	return nil
}
