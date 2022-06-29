//go:build !leveldb
// +build !leveldb

package shed

import "github.com/gauss-project/aurorafs/pkg/shed/wiredtiger"

const WIREDTIGER = "wiredtiger"

func init() {
	Register(WIREDTIGER, wiredtiger.Driver{})
}
