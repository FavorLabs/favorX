package shed

import "github.com/gauss-project/aurorafs/pkg/shed/leveldb"

const LEVELDB = "leveldb"

func init() {
	Register(LEVELDB, leveldb.Driver{})
}
