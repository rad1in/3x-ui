//go:build windows

package l2tp

import "errors"

func terminate(int) error {
	return errors.New("l2tp links are not supported on windows")
}
