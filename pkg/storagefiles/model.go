package storagefiles

import (
	"github.com/FavorLabs/favorX/pkg/boson"
)

type UploadRequest struct {
	Source boson.Address
	Hash   boson.Address
}
