package storagefiles

import (
	"github.com/FavorLabs/favorX/pkg/boson"
)

type Config struct {
	ApiGateway         string
	Overlay            boson.Address
	Gid                boson.Address
	GroupName          string
	SkipOracleRegister bool
	Capacity           uint64 // total disk size
	CacheDir           string // cache task dir
	DelFileTime        int64  // interval del file time unit Minute
	BlockSize          int
	RetryNumber        int
	Redundant          uint64
	DataDir            string
}
