package api

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"github.com/FavorLabs/favorX/pkg/chunkinfo"
	"github.com/FavorLabs/favorX/pkg/fileinfo"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/FavorLabs/favorX/pkg/file"
	"github.com/FavorLabs/favorX/pkg/file/loadsave"
	"github.com/FavorLabs/favorX/pkg/manifest"
	"github.com/FavorLabs/favorX/pkg/sctx"
	"github.com/gauss-project/aurorafs/pkg/boson"
	"github.com/gauss-project/aurorafs/pkg/jsonhttp"
	"github.com/gauss-project/aurorafs/pkg/logging"
	"github.com/gauss-project/aurorafs/pkg/tracing"
	"github.com/gorilla/mux"
)

type dirs struct {
	Dirs []string `json:"dirs"`
}

// dirUploadHandler uploads a directory supplied as a tar in an HTTP request
func (s *server) dirUploadHandler(w http.ResponseWriter, r *http.Request) {
	logger := tracing.NewLoggerWithTraceID(r.Context(), s.logger)
	if r.Body == http.NoBody {
		logger.Error("dir upload dir: request has no body")
		jsonhttp.BadRequest(w, invalidRequest)
		return
	}
	contentType := r.Header.Get(contentTypeHeader)
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		logger.Errorf("dir upload dir: invalid content-type")
		logger.Debugf("dir upload dir: invalid content-type err: %v", err)
		jsonhttp.BadRequest(w, invalidContentType)
		return
	}

	var dReader dirReader
	switch mediaType {
	case contentTypeTar:
		dReader = &tarReader{r: tar.NewReader(r.Body), logger: s.logger}
	case multiPartFormData:
		dReader = &multipartReader{r: multipart.NewReader(r.Body, params["boundary"])}
	default:
		logger.Error("dir upload dir: invalid content-type for directory upload")
		jsonhttp.BadRequest(w, invalidContentType)
		return
	}
	defer r.Body.Close()

	ctx := r.Context()

	p := requestPipelineFn(s.storer, r)
	factory := requestPipelineFactory(ctx, s.storer, r)
	reference, err := storeDir(
		ctx,
		requestEncrypt(r),
		dReader,
		s.logger,
		p,
		loadsave.New(s.storer, factory),
		s.fileInfo,
		s.chunkInfo,
		r.Header.Get(CollectionNameHeader),
		r.Header.Get(IndexDocumentHeader),
		r.Header.Get(ErrorDocumentHeader),
		r.Header.Get(ReferenceLinkHeader),
	)
	if err != nil {
		logger.Debugf("dir upload dir: store dir err: %v", err)
		logger.Errorf("dir upload dir: store dir")
		jsonhttp.InternalServerError(w, directoryStoreError)
		return
	}

	if strings.ToLower(r.Header.Get(PinHeader)) == StringTrue {
		if err := s.pinning.CreatePin(r.Context(), reference, false); err != nil {
			logger.Debugf("dir upload dir: creation of pin for %q failed: %v", reference, err)
			logger.Error("dir upload dir: creation of pin failed")
			jsonhttp.InternalServerError(w, nil)
			return
		}
		err = s.fileInfo.PinFile(reference, true)
		if err != nil {
			s.logger.Errorf("dir upload file:update fileinfo pin failed:%v", err)
		}
	}
	err = s.fileInfo.AddFile(reference)
	if err != nil {
		jsonhttp.NotFound(w, "add file error")
		return
	}
	jsonhttp.Created(w, UploadResponse{
		Reference: reference,
	})
}

// UnescapeUnicode convert the raw Unicode encoding to character
func UnescapeUnicode(raw string) (string, error) {
	str, err := strconv.Unquote(strings.Replace(strconv.Quote(raw), `\\u`, `\u`, -1))
	if err != nil {
		return "", err
	}
	return str, nil
}

// storeDir stores all files recursively contained in the directory given as a tar/multipart
// it returns the hash for the uploaded manifest corresponding to the uploaded dir
func storeDir(
	ctx context.Context,
	encrypt bool,
	reader dirReader,
	log logging.Logger,
	p pipelineFunc,
	ls file.LoadSaver,
	fileInfo fileinfo.Interface,
	chunkInfo chunkinfo.Interface,
	dirName,
	indexFilename,
	errorFilename,
	referenceLink string,
) (boson.Address, error) {
	logger := tracing.NewLoggerWithTraceID(ctx, log)

	dirManifest, err := manifest.NewDefaultManifest(ls, encrypt)
	if err != nil {
		return boson.ZeroAddress, err
	}

	if indexFilename != "" && strings.ContainsRune(indexFilename, '/') {
		return boson.ZeroAddress, fmt.Errorf("index document suffix must not include slash character")
	}

	if dirName != "" && strings.ContainsRune(dirName, '/') {
		return boson.ZeroAddress, fmt.Errorf("dir name must not include slash character")
	}

	filesAdded := 0

	// iterate through the files in the supplied tar
	for {
		fileInfo, err := reader.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return boson.ZeroAddress, fmt.Errorf("read tar stream: %w", err)
		}

		fileReference, err := p(ctx, fileInfo.Reader)
		if err != nil {
			return boson.ZeroAddress, fmt.Errorf("store dir file: %w", err)
		}
		logger.Tracef("uploaded dir file %v with reference %v", fileInfo.Path, fileReference)

		fileMetadata := map[string]string{
			manifest.EntryMetadataContentTypeKey: fileInfo.ContentType,
			manifest.EntryMetadataFilenameKey:    fileInfo.Name,
		}
		// add file entry to dir manifest
		err = dirManifest.Add(ctx, fileInfo.Path, manifest.NewEntry(fileReference, fileMetadata, 0))
		if err != nil {
			return boson.ZeroAddress, fmt.Errorf("add to manifest: %w", err)
		}

		filesAdded++
	}

	// check if files were uploaded through the manifest
	if filesAdded == 0 {
		return boson.ZeroAddress, fmt.Errorf("no files in tar")
	}

	// store website information
	if dirName != "" || indexFilename != "" || errorFilename != "" {
		metadata := map[string]string{}
		if dirName != "" {
			realDirName, err := UnescapeUnicode(dirName)
			if err != nil {
				return boson.ZeroAddress, err
			}
			metadata[manifest.EntryMetadataDirnameKey] = realDirName
		}
		if indexFilename != "" {
			realIndexFilename, err := UnescapeUnicode(indexFilename)
			if err != nil {
				return boson.ZeroAddress, err
			}
			metadata[manifest.WebsiteIndexDocumentSuffixKey] = realIndexFilename
		}
		if errorFilename != "" {
			realErrorFilename, err := UnescapeUnicode(errorFilename)
			if err != nil {
				return boson.ZeroAddress, err
			}
			metadata[manifest.WebsiteErrorDocumentPathKey] = realErrorFilename
		}
		if referenceLink != "" {
			metadata[manifest.ReferenceLinkKey] = referenceLink
		}
		rootManifestEntry := manifest.NewEntry(boson.ZeroAddress, metadata, 0)
		err = dirManifest.Add(ctx, manifest.RootPath, rootManifestEntry)
		if err != nil {
			return boson.ZeroAddress, fmt.Errorf("add to manifest: %w", err)
		}
	}

	var storeSizeFn []manifest.StoreSizeFunc

	// save manifest
	manifestReference, err := dirManifest.Store(ctx, storeSizeFn...)
	if err != nil {
		return boson.ZeroAddress, fmt.Errorf("store manifest: %w", err)
	}
	fn := func(nodeType int, path, prefix, hash []byte, metadata map[string]string) error {
		if nodeType == 0 {
			if strings.Contains(string(prefix), "._") {
				return nil
			}
			fcid := boson.NewAddress(hash)
			bitLen, err := fileInfo.GetFileSize(fcid)
			if err != nil {
				return err
			}
			if bitLen > 1 {
				bitLen++
			}
			err = chunkInfo.OnFileUpload(ctx, fcid, bitLen)
			if err != nil {
				return err
			}
		}
		return nil
	}
	ctx = sctx.SetRootHash(ctx, manifestReference)
	err = dirManifest.IterateDirectories(ctx, []byte(""), 0, fn)
	if err != nil {
		return boson.ZeroAddress, err
	}
	//err = chunkInfo.OnFileUpload(ctx, manifestReference, int64(bitLen))
	//if err != nil {
	//	return boson.ZeroAddress, err
	//}
	logger.Tracef("finished uploaded dir with reference %v", manifestReference)

	return manifestReference, nil
}

type FileInfo struct {
	Path        string
	Name        string
	ContentType string
	Size        int64
	Reader      io.Reader
}

type dirReader interface {
	Next() (*FileInfo, error)
}

type tarReader struct {
	r      *tar.Reader
	logger logging.Logger
}

func (t *tarReader) Next() (*FileInfo, error) {
	for {
		fileHeader, err := t.r.Next()
		if err != nil {
			return nil, err
		}

		fileName := fileHeader.FileInfo().Name()
		contentType := mime.TypeByExtension(filepath.Ext(fileHeader.Name))
		fileSize := fileHeader.FileInfo().Size()
		filePath := filepath.Clean(fileHeader.Name)

		if filePath == "." {
			t.logger.Warning("skipping file upload empty path")
			continue
		}
		if runtime.GOOS == "windows" {
			// always use Unix path separator
			filePath = filepath.ToSlash(filePath)
		}
		// only store regular files
		if !fileHeader.FileInfo().Mode().IsRegular() {
			t.logger.Warningf("skipping file upload for %s as it is not a regular file", filePath)
			continue
		}

		return &FileInfo{
			Path:        filePath,
			Name:        fileName,
			ContentType: contentType,
			Size:        fileSize,
			Reader:      t.r,
		}, nil
	}
}

// multipart reader returns files added as a multipart form. We will ensure all the
// part headers are passed correctly
type multipartReader struct {
	r *multipart.Reader
}

func (m *multipartReader) Next() (*FileInfo, error) {
	part, err := m.r.NextPart()
	if err != nil {
		return nil, err
	}

	fileName := part.FileName()
	if fileName == "" {
		fileName = part.FormName()
	}
	if fileName == "" {
		return nil, errors.New("filename missing")
	}

	contentType := part.Header.Get(contentTypeHeader)
	if contentType == "" {
		return nil, errors.New("content-type missing")
	}

	contentLength := part.Header.Get("Content-Length")
	if contentLength == "" {
		return nil, errors.New("content-length missing")
	}
	fileSize, err := strconv.ParseInt(contentLength, 10, 64)
	if err != nil {
		return nil, errors.New("invalid file size")
	}

	if filepath.Dir(fileName) != "." {
		return nil, errors.New("multipart upload supports only single directory")
	}

	return &FileInfo{
		Path:        fileName,
		Name:        fileName,
		ContentType: contentType,
		Size:        fileSize,
		Reader:      part,
	}, nil
}

func (s *server) fileDeleteHandler(w http.ResponseWriter, r *http.Request) {
	addr := mux.Vars(r)["address"]
	hash, err := boson.ParseHexAddress(addr)
	if err != nil {
		s.logger.Debugf("dir delete: parse address: %w", err)
		s.logger.Errorf("dir delete: parse address %s", addr)
		jsonhttp.BadRequest(w, "invalid address")
		return
	}

	err = s.fileInfo.DeleteFile(hash)
	s.chunkInfo.CancelFindChunkInfo(hash)
	if err != nil {
		s.logger.Errorf("dir delete: remove file: %w", err)
		jsonhttp.InternalServerError(w, "dir deleting occur error")
		return
	}

	jsonhttp.OK(w, nil)
}
