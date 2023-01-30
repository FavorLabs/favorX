package fileinfo

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/FavorLabs/favorX/pkg/address"
	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/localstore"
	"github.com/FavorLabs/favorX/pkg/localstore/chunkstore"
	"github.com/FavorLabs/favorX/pkg/localstore/filestore"
	"github.com/FavorLabs/favorX/pkg/logging"
	"github.com/FavorLabs/favorX/pkg/resolver"
	"github.com/FavorLabs/favorX/pkg/storage"
)

type FileView struct {
	filestore.FileView
	BvLen int
	Bv    []byte
}

type ChunkInfoSource struct {
	Len         int                      `json:"len"`
	ChunkSource []address.ChunkSourceApi `json:"chunkSource"`
}

type Interface interface {
	GetFileView(rootCid boson.Address) (filestore.FileView, error)
	GetFileList(page filestore.Page, filter []filestore.Filter, sort filestore.Sort) ([]FileView, int)
	GetFileSize(rootCid boson.Address) (int64, error)
	ManifestView(ctx context.Context, nameOrHex string, pathVar string, depth int) (*ManifestNode, error)
	AddFile(rootCid boson.Address) error
	DeleteFile(rootCid boson.Address) error
	PinFile(rootCid boson.Address, pinned bool) error
	RegisterFile(rootCid boson.Address, registered bool) error
	GetChunkInfoDiscoverOverlays(rootCid boson.Address) []address.ChunkInfoOverlay
	GetChunkInfoServerOverlays(rootCid boson.Address) []address.ChunkInfoOverlay
	GetChunkInfoSource(rootCid boson.Address) ChunkInfoSource
	AddFileMirror(next, rootCid boson.Address, ope filestore.Operation) error
	FileCounter(rootCid boson.Address) error
}

type FileInfo struct {
	addr       boson.Address
	localStore *localstore.DB
	logger     logging.Logger
	resolver   resolver.Interface
}

func New(addr boson.Address, db *localstore.DB, logger logging.Logger, resolver resolver.Interface) Interface {
	return &FileInfo{
		addr:       addr,
		localStore: db,
		logger:     logger,
		resolver:   resolver,
	}
}

func (f *FileInfo) GetFileSize(rootCid boson.Address) (int64, error) {
	ctx := context.TODO()
	chunk, err := f.localStore.Get(ctx, storage.ModeGetRequest, rootCid, 0)
	if err != nil {
		return 0, err
	}
	var chunkData = chunk.Data()
	span := int64(binary.LittleEndian.Uint64(chunkData[:boson.SpanSize]))
	size := chunkLen(span)
	return size, nil
}

func (f *FileInfo) GetFileView(rootCid boson.Address) (filestore.FileView, error) {
	v, ok := f.localStore.GetFile(rootCid)
	if !ok {
		return filestore.FileView{}, fmt.Errorf("fileStore get %s:fileinfo not found", rootCid.String())
	}
	return v, nil
}

func (f *FileInfo) GetFileList(page filestore.Page, filter []filestore.Filter, sort filestore.Sort) ([]FileView, int) {
	list, total := f.localStore.GetListFile(page, filter, sort)
	fileList := make([]FileView, 0, len(list))
	for index := 0; index < len(list); index++ {
		fv := FileView{
			FileView: list[index],
		}
		consumerList, err := f.localStore.GetChunk(chunkstore.SERVICE, list[index].RootCid)
		if err != nil {
			f.logger.Errorf("fileInfo GetFileList:%w", err)
		} else {
			for cIndex := 0; cIndex < len(consumerList); cIndex++ {
				if consumerList[cIndex].Overlay.Equal(f.addr) {
					fv.Bv = consumerList[cIndex].B
					fv.BvLen = consumerList[cIndex].Len
					break
				}
			}
		}
		fileList = append(fileList, fv)
	}
	return fileList, total
}

func (f *FileInfo) AddFile(rootCid boson.Address) error {
	if f.localStore.HasFile(rootCid) {
		return nil
	}
	manifest, err := f.ManifestView(context.TODO(), rootCid.String(), defaultPathVar, defaultDepth)
	if err != nil {
		return fmt.Errorf("fileStore get manifest:%s", err.Error())
	}
	if manifest.Nodes != nil {
		var fileSize uint64
		var maniFestNode ManifestNode
		for _, mv := range manifest.Nodes {
			fileSize = fileSize + mv.Size
			maniFestNode = *mv
		}
		manifest.Type = maniFestNode.Type
		manifest.Hash = maniFestNode.Hash
		manifest.Size = fileSize
		manifest.Extension = maniFestNode.Extension
		manifest.MimeType = maniFestNode.MimeType
	}
	fileInfo := filestore.FileView{
		RootCid:       rootCid,
		Pinned:        false,
		Registered:    false,
		Size:          int(manifest.Size),
		Default:       manifest.Default,
		ErrDefault:    manifest.ErrDefault,
		Type:          manifest.Type,
		Name:          manifest.Name,
		Extension:     manifest.Extension,
		MimeType:      manifest.MimeType,
		ReferenceLink: manifest.ReferenceLink,
	}

	err = f.localStore.PutFile(fileInfo)
	if err != nil {
		return fmt.Errorf("fileStore put new fileinfo %s:%s", rootCid.String(), err.Error())
	}
	return nil
}

func (f *FileInfo) DeleteFile(rootCid boson.Address) error {
	if err := f.localStore.DeleteFile(rootCid); err != nil {
		return err
	}
	return f.DeletedFileMirror(rootCid)
}

func (f *FileInfo) PinFile(rootCid boson.Address, pinned bool) error {
	file, ok := f.localStore.GetFile(rootCid)
	if !ok {
		return fmt.Errorf("fileStore update fileinfo %s:fileinfo not found", rootCid.String())
	}
	file.Pinned = pinned
	err := f.localStore.PutFile(file)
	if err != nil {
		return fmt.Errorf("fileStore put new fileinfo %s:%s", rootCid.String(), err.Error())
	}
	return nil
}

func (f *FileInfo) RegisterFile(rootCid boson.Address, registered bool) error {
	file, ok := f.localStore.GetFile(rootCid)
	if !ok {
		return fmt.Errorf("fileStore update fileinfo %s:fileinfo not found", rootCid.String())
	}
	file.Registered = registered
	err := f.localStore.PutFile(file)
	if err != nil {
		return fmt.Errorf("fileStore put new fileinfo %s:%s", rootCid.String(), err.Error())
	}
	return nil
}

func (f *FileInfo) GetChunkInfoDiscoverOverlays(rootCid boson.Address) []address.ChunkInfoOverlay {
	res := make([]address.ChunkInfoOverlay, 0)
	consumerList, err := f.localStore.GetChunk(chunkstore.DISCOVER, rootCid)
	if err != nil {
		f.logger.Errorf("fileInfo GetChunkInfoDiscoverOverlays:%w", err)
		return res
	}
	for _, c := range consumerList {
		bv := address.BitVectorApi{B: c.B, Len: c.Len}
		cio := address.ChunkInfoOverlay{Overlay: c.Overlay.String(), Bit: bv}
		res = append(res, cio)
	}
	return res
}

func (f *FileInfo) GetChunkInfoServerOverlays(rootCid boson.Address) []address.ChunkInfoOverlay {
	res := make([]address.ChunkInfoOverlay, 0)
	consumerList, err := f.localStore.GetChunk(chunkstore.SERVICE, rootCid)
	if err != nil {
		f.logger.Errorf("fileInfo GetChunkInfoServerOverlays:%w", err)
		return res
	}
	for _, c := range consumerList {
		bv := address.BitVectorApi{Len: c.Len, B: c.B}
		cio := address.ChunkInfoOverlay{Overlay: c.Overlay.String(), Bit: bv}
		res = append(res, cio)
	}
	return res
}

func (f *FileInfo) GetChunkInfoSource(rootCid boson.Address) ChunkInfoSource {
	var res ChunkInfoSource
	consumerList, err := f.localStore.GetChunk(chunkstore.SOURCE, rootCid)
	if err != nil {
		f.logger.Errorf("fileInfo GetChunkInfoSource:%w", err)
		return res
	}

	for _, c := range consumerList {
		chunkBit := address.BitVectorApi{
			Len: c.Len,
			B:   c.B,
		}
		source := address.ChunkSourceApi{
			Overlay:  c.Overlay.String(),
			ChunkBit: chunkBit,
		}
		res.ChunkSource = append(res.ChunkSource, source)
	}
	c, err := f.localStore.GetChunkByOverlay(chunkstore.SERVICE, rootCid, f.addr)
	if err != nil {
		f.logger.Errorf("fileInfo GetChunkInfoSource:%w", err)
		return res
	}
	res.Len = c.Len
	return res
}

func (f *FileInfo) FileCounter(rootCid boson.Address) error {
	return f.localStore.ChunkCounter(rootCid)
}

func chunkLen(span int64) int64 {
	count := span / boson.ChunkSize
	count1 := span % boson.ChunkSize
	if count1 > 0 && span < boson.ChunkSize {
		return 0
	}
	if count1 > 0 {
		count++
	}
	if count > boson.Branches {
		count += chunkLen1(count)
	}
	return count
}

func chunkLen1(count int64) int64 {
	count1 := count / boson.Branches
	if count1 == 0 {
		return 0
	}
	count2 := count % boson.Branches
	count = count2 + count1
	if count == 1 {
		return count
	}
	count = chunkLen1(count)
	return count1 + count
}
