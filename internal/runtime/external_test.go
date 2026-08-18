package runtime_test

import (
	"errors"

	"github.com/limecloud/contentcloud/internal/platform/fault"
)

func hasDomainCode(err error, code string) bool {
	var value *fault.Error
	return errors.As(err, &value) && value.Code == code
}
