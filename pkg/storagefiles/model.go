package storagefiles

import (
	"github.com/FavorLabs/favorX/pkg/address"
)

type UploadRequest struct {
	Source string `json:"source"`
	Hash   string `json:"hash"`
	Force  bool   `json:"force"`
	Sync   bool   `json:"sync,omitempty"`
}

type UploadResponse struct {
	WorkerID uint64               `json:"workerID"`
	Message  string               `json:"message"`
	Vector   address.BitVectorApi `json:"vector"`
}

type GroupMessage struct {
	GID       string `json:"gid"`
	Data      string `json:"data"`
	From      string `json:"from"`
	SessionID string `json:"sessionID"`
}

type GroupMessageReply struct {
	SessionID string `json:"sessionID"`
	Data      []byte
	ErrCh     chan error
}
