package diags

import (
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

func Handle(diags diag.Diagnostics, logFunc func(string, ...any)) error {
	var errs error

	for _, d := range diags {
		msg := d.Summary()
		if d.Detail() != "" {
			msg = msg + " | " + d.Detail()
		}

		switch d.Severity() {
		case diag.SeverityError:
			errs = errors.Join(errs, errors.New(msg))
		case diag.SeverityWarning:
			logFunc(msg)
		default:
			errs = errors.Join(errs, fmt.Errorf("unhandled diagnostic severity %d - %q", d.Severity(), msg))
		}
	}

	return errs
}
