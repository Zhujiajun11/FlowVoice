//go:build !windows

package desktop

import "errors"

func New(uiURL string) (Integration, error) {
	_ = uiURL
	return nil, errors.New("desktop mode is only implemented on Windows")
}
