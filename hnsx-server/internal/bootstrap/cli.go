package bootstrap

type CLIError struct {
	Code int
	Err error
}

func (ce *CLIError) Error() string {
	return ce.Err.Error()
}