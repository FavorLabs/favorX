package fileinfo

import (
	"github.com/FavorLabs/favorX/pkg/localstore/filestore"
	"github.com/gauss-project/aurorafs/pkg/boson"
)

func (f *FileInfo) AddFileMirror(next, rootCid boson.Address, ope filestore.Operation) error {
	err := f.localStore.PutMirrorFile(next, rootCid, ope)
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
