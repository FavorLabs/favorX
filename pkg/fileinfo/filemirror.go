package fileinfo

import (
	"github.com/FavorLabs/favorX/pkg/localstore/filestore"
	"github.com/FavorLabs/favorX/pkg/storage"
	"github.com/gauss-project/aurorafs/pkg/boson"
)

func (f *FileInfo) AddFileMirror(next boson.Address, ope filestore.Operation, rootCid boson.Address) error {
	file, ok := f.localStore.GetFile(rootCid)
	if !ok {
		return ErrNotFound
	}
	err := f.localStore.DeleteFile(rootCid)
	if err != nil {
		return err
	}
	m, err := f.localStore.GetMirror(rootCid)
	pre := boson.ZeroAddress
	if err != nil {
		if err != storage.ErrNotFound {
			return err
		}
	} else {
		pre = m.RootCid
	}
	err = f.localStore.PutMirrorFile(pre, next, ope, file)
	if err != nil {
		return err
	}
	if err = f.AddFile(next); err != nil {
		return err
	}
	return nil
}

func (f *FileInfo) DeletedFileMirror(reference boson.Address) error {
	mirrors, err := f.localStore.GetMirrors(reference)
	if err != nil {
		return err
	}
	for _, m := range mirrors {
		_ = f.localStore.DeleteMirror(m.NextRootCid)
	}
	return nil
}

func (f *FileInfo) Rollback(reference, rollback boson.Address) error {
	err := f.DeleteFile(reference)
	if err != nil {
		return err
	}
	if err = f.AddFile(rollback); err != nil {
		return err
	}

	return nil
}
