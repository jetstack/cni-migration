package app

import (
	"context"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	// Load all auth plugins
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	cliflag "k8s.io/component-base/cli/flag"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	"github.com/joshvanl/cni-migration/pkg"
	"github.com/joshvanl/cni-migration/pkg/cleanup"
	"github.com/joshvanl/cni-migration/pkg/config"
	"github.com/joshvanl/cni-migration/pkg/migrate"
	"github.com/joshvanl/cni-migration/pkg/preflight"
	"github.com/joshvanl/cni-migration/pkg/prepare"
	"github.com/joshvanl/cni-migration/pkg/priority"
	"github.com/joshvanl/cni-migration/pkg/roll"
)

type NewFunc func(context.Context, *config.Config) pkg.Step
type ReadyFunc func() (bool, error)
type RunFunc func(bool) error

type Options struct {
	NoDryRun   bool
	LogLevel   string
	ConfigPath string

	StepAll               bool
	StepPreflight         bool
	StepPrepare           bool
	StepRollNodes         bool
	StepChangeCNIPriority bool
	StepMigrateNode       string
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

			if len(o.StepMigrateNode) > 0 {
				ctx = context.WithValue(ctx, migrate.ContextNodeKey, o.StepMigrateNode)
			}

			config, err := config.New(o.ConfigPath, lvl, factory)
			if err != nil {
				return fmt.Errorf("failed to build config: %s", err)
			}

			if err := run(ctx, config, o); err != nil {
				config.Log.Error(err)
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

func run(ctx context.Context, config *config.Config, o *Options) error {
	dryrun := !o.NoDryRun

	if dryrun {
		config.Log = config.Log.WithField("dry-run", "true")
	}

	var steps []pkg.Step
	for _, f := range []NewFunc{
		preflight.New,
		prepare.New,
		roll.New,
		priority.New,
		migrate.New,
		cleanup.New,
	} {
		steps = append(steps, f(ctx, config))
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
		o.StepPreflight,
		o.StepPrepare,
		o.StepRollNodes,
		o.StepChangeCNIPriority,
		(len(o.StepMigrateNode) > 0 || o.StepMigrateAllNodes),
		o.StepCleanUp,
	}

	maxStep := -1
	for i, b := range stepBool {
		if b {
			maxStep = i
		}
	}

	if maxStep == -1 {
		config.Log.Info("no steps specified")
		return nil
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
				return fmt.Errorf("step %d failed: %s", i, err)
			}

			if !ready {
				return fmt.Errorf("step %d not ready...", i)
			}
		}
	}

	config.Log.Info("steps successful.")

	return nil
}
