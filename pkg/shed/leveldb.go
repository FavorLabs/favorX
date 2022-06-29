//go:build leveldb
// +build leveldb

package shed

import "github.com/FavorLabs/favorX/pkg/shed/leveldb"

const LEVELDB = "leveldb"

var TestDriver = LEVELDB

func init() {
	Register(LEVELDB, leveldb.Driver{})
}
