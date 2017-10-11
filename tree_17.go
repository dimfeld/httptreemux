// +build !go1.8

package httptreemux

//Break out unescapePath to use url.PathUnescape in go 1.8+

import "net/url"

func unescapePath(path string) (string, error) {
	return url.QueryUnescape(path)
}
