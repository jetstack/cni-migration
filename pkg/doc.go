package pkg

type Step interface {
	Ready() (bool, error)
	Run(dryrun bool) error
}
