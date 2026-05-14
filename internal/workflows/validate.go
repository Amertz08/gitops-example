package workflows

import "go.temporal.io/sdk/temporal"

type validator interface {
	validate() error
}

func invalidInput(v validator) error {
	if err := v.validate(); err != nil {
		return temporal.NewNonRetryableApplicationError(err.Error(), "InvalidInput", err)
	}
	return nil
}
