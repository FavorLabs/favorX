package shed

import "github.com/FavorLabs/favorX/pkg/shed/leveldb"

const LEVELDB = "leveldb"

func init() {
	Register(LEVELDB, leveldb.Driver{})
}
