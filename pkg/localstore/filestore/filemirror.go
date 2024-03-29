package filestore

import (
	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/storage"
)

type FileMirror struct {
	PreRootCid    boson.Address
	NextRootCid   boson.Address
	RootCid       boson.Address
	Operation     Operation
	Hash          string
	Pinned        bool
	Registered    bool
	Size          int
	Type          string
	Name          string
	Extension     string
	Default       string
	ErrDefault    string
	MimeType      string
	ReferenceLink string
}

type Operation int

const (
	REMOVE Operation = iota
	MOVE
	COPY
	MKDIR
)

func (o Operation) String() string {
	switch o {
	case REMOVE:
		return "REMOVE"
	case MOVE:
		return "MOVE"
	case COPY:
		return "COPY"
	case MKDIR:
		return "MKDIR"
	default:
		return "Unknown"
	}
}

func (fs *fileStore) putMirror(pre, next boson.Address, ope Operation, file FileView) error {

	has, err := fs.hasMirror(next)
	if err != nil || has {
		return err
	}
	fileMirror := FileMirror{
		PreRootCid:    pre,
		NextRootCid:   next,
		RootCid:       file.RootCid,
		Operation:     ope,
		Hash:          file.Hash,
		Pinned:        file.Pinned,
		Registered:    file.Registered,
		Size:          file.Size,
		Type:          file.Type,
		Name:          file.Name,
		Extension:     file.Extension,
		Default:       file.Default,
		ErrDefault:    file.ErrDefault,
		MimeType:      file.MimeType,
		ReferenceLink: file.ReferenceLink,
	}
	if err := fs.stateStore.Put(mirrorPrefix+"-"+next.String(), fileMirror); err != nil {
		return err
	}
	return nil
}

func (fs *fileStore) delMirror(reference boson.Address) error {
	if err := fs.stateStore.Delete(mirrorPrefix + "-" + reference.String()); err != nil {
		return err
	}
	return nil
}

func (fs *fileStore) getMirrors(reference boson.Address) (fms []*FileMirror, err error) {
	fm, err := fs.getMirror(reference)
	if err != nil {
		if err == storage.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	fms = append(fms, fm)
	if fm.PreRootCid.Equal(boson.ZeroAddress) {
		return fms, nil
	}

	nfm, err := fs.getMirrors(fm.RootCid)
	if err != nil {
		return nil, err
	}
	fms = append(fms, nfm...)
	return fms, nil
}

func (fs *fileStore) getMirror(reference boson.Address) (*FileMirror, error) {
	var fm FileMirror
	if err := fs.stateStore.Get(mirrorPrefix+"-"+reference.String(), &fm); err != nil {
		return nil, err
	}
	return &fm, nil
}

func (fs *fileStore) hasMirror(reference boson.Address) (bool, error) {
	_, err := fs.getMirror(reference)
	if err != nil {
		if err == storage.ErrNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
