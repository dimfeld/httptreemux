//go:build !go1.8
// +build !go1.8

package httptreemux

import "net/url"

func unescape(path string) (string, error) {
	return url.QueryUnescape(path)
}
