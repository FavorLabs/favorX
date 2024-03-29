package filestore

import (
	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/storage"
	"strings"
)

type Interface interface {
	Init() error
	Get(reference boson.Address) (FileView, bool)
	GetMirror(reference boson.Address) (*FileMirror, error)
	GetList(page Page, filter []Filter, sort Sort) ([]FileView, int)
	GetMirrors(reference boson.Address) (fms []*FileMirror, err error)
	Put(file FileView) error
	PutMirror(pre, next boson.Address, ope Operation, file FileView) error
	Delete(reference boson.Address) error
	DeleteMirror(reference boson.Address) error
	Has(reference boson.Address) bool
	Update(file FileView) error
}
type fileStore struct {
	stateStore storage.StateStorer
	files      map[string]FileView
}

type FileView struct {
	RootCid       boson.Address
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

type Page struct {
	PageNum  int
	PageSize int
}

type Filter struct {
	Key   string
	Term  string
	Value string
}

type Sort struct {
	Key   string
	Order string
}

var keyPrefix = "file"
var mirrorPrefix = "mirror"

func New(storer storage.StateStorer) Interface {
	return &fileStore{
		stateStore: storer,
		files:      make(map[string]FileView),
	}
}

func (fs *fileStore) Init() error {
	if err := fs.stateStore.Iterate(keyPrefix, func(k, v []byte) (bool, error) {
		if !strings.HasPrefix(string(k), keyPrefix) {
			return true, nil
		}
		key := string(k)
		var fv FileView
		if err := fs.stateStore.Get(key, &fv); err != nil {
			return false, err
		}
		fs.put(fv)
		return false, nil
	}); err != nil {
		return err
	}
	return nil
}

func (fs *fileStore) Get(reference boson.Address) (FileView, bool) {
	file, ok := fs.files[reference.String()]
	return file, ok
}

func (fs *fileStore) GetList(page Page, filter []Filter, sort Sort) ([]FileView, int) {
	ff := filterFile(fs.files, filter)
	sf := sortFile(ff, sort.Key, sort.Order)
	pf := pageFile(sf, page)
	total := len(ff)
	return pf, total
}

func (fs *fileStore) GetMirror(reference boson.Address) (*FileMirror, error) {
	return fs.getMirror(reference)
}

func (fs *fileStore) GetMirrors(reference boson.Address) (fms []*FileMirror, err error) {
	return fs.getMirrors(reference)
}

func (fs *fileStore) Update(file FileView) error {
	fs.files[file.RootCid.String()] = file
	if err := fs.stateStore.Put(keyPrefix+"-"+file.RootCid.String(), file); err != nil {
		return err
	}
	return nil
}

func (fs *fileStore) Put(file FileView) error {
	fs.files[file.RootCid.String()] = file
	if err := fs.stateStore.Put(keyPrefix+"-"+file.RootCid.String(), file); err != nil {
		return err
	}
	return nil
}

func (fs *fileStore) PutMirror(pre, next boson.Address, ope Operation, file FileView) error {
	return fs.putMirror(pre, next, ope, file)
}

func (fs *fileStore) put(file FileView) {
	exists := fs.Has(file.RootCid)
	if exists {
		return
	}
	fs.files[file.RootCid.String()] = file
}

func (fs *fileStore) Delete(reference boson.Address) error {
	exists := fs.Has(reference)
	if !exists {
		return nil
	}
	delete(fs.files, reference.String())
	if err := fs.stateStore.Delete(keyPrefix + "-" + reference.String()); err != nil {
		return err
	}
	return nil
}
func (fs *fileStore) DeleteMirror(reference boson.Address) error {
	return fs.delMirror(reference)
}
func (fs *fileStore) Has(reference boson.Address) bool {
	_, ok := fs.files[reference.String()]
	return ok
}
