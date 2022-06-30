//go:build !leveldb
// +build !leveldb

package shed

import "github.com/FavorLabs/favorX/pkg/shed/wiredtiger"

const WIREDTIGER = "wiredtiger"

func init() {
	Register(WIREDTIGER, wiredtiger.Driver{})
}
