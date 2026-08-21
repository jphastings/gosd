// Package kernelparam validates the extra kernel command-line parameters a
// developer bakes into an image with `gosd build --kernel-param`.
//
// The check is on shape, never vocabulary. gosd cannot know which parameters
// a board's kernel understands - the set changes per board, and changes again
// the moment somebody compiles a driver in with `gosd build-kernel` - so an
// allow-list of "known good" parameters would go stale and block legitimate
// use. A parameter this package accepts but the kernel has never heard of is
// inert, exactly as an unknown parameter on any Linux command line is.
//
// What gosd does know is what would corrupt the boot config it writes: the Pi
// family's single-line cmdline.txt and the mainline fleet's extlinux.conf
// `append` line both separate parameters with spaces and end at a newline. So
// a value that would silently split into two parameters, truncate the line,
// or land an unprintable byte in a text file is refused, and everything else
// is passed through untouched.
package kernelparam

import (
	"errors"
	"fmt"
	"strings"
)

// Parse validates each --kernel-param value and returns the accepted
// parameters in the order they were given, which is also the order boards
// render them in.
//
// Given order is deliberately preserved rather than sorted: it makes a build
// byte-reproducible for a given command line either way, but only this way
// does the kernel's last-one-wins handling of a repeated parameter stay under
// the developer's control instead of gosd's. A nil/empty input returns nil,
// so a board renders exactly what it rendered before the flag existed.
func Parse(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}

	params := make([]string, 0, len(values))
	for _, v := range values {
		if err := validate(v); err != nil {
			return nil, err
		}
		params = append(params, v)
	}
	return params, nil
}

// example is named in every error so the message shows the shape being asked
// for rather than only the shape that was rejected.
const example = "--kernel-param snd_bcm2835.enable_hdmi=1"

func validate(v string) error {
	if v == "" {
		return errors.New("--kernel-param was given an empty value; pass a kernel command-line parameter, e.g. " + example +
			", or a bare switch such as --kernel-param nomodeset")
	}

	for i := 0; i < len(v); i++ {
		switch b := v[i]; {
		case b == ' ' || b == '\t':
			return fmt.Errorf("--kernel-param %q is invalid: a kernel parameter cannot contain whitespace, because the kernel command line separates parameters with spaces; give each parameter its own --kernel-param flag", v)
		case b == '\n' || b == '\r':
			return fmt.Errorf("--kernel-param %q is invalid: a newline would truncate the boot config gosd writes (cmdline.txt, extlinux.conf); give each parameter its own --kernel-param flag", v)
		case b == 0x00:
			return fmt.Errorf("--kernel-param %q is invalid: it contains a NUL byte, which the boot config gosd writes cannot carry", v)
		case b < 0x20 || b == 0x7f:
			return fmt.Errorf("--kernel-param %q is invalid: it contains the control character %#02x, which the boot config gosd writes cannot carry", v, b)
		}
	}

	if strings.HasPrefix(v, "=") {
		return fmt.Errorf("--kernel-param %q is invalid: it has no parameter name before the %q; write it as name=value, e.g. %s", v, "=", example)
	}

	return nil
}
