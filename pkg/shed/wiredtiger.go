//go:build wiredtiger
// +build wiredtiger

package shed

import "github.com/FavorLabs/favorX/pkg/shed/wiredtiger"

const WIREDTIGER = "wiredtiger"

var TestDriver = WIREDTIGER

func init() {
	Register(WIREDTIGER, wiredtiger.Driver{})
}
