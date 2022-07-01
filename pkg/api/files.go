package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/FavorLabs/favorX/pkg/file/joiner"
	"github.com/FavorLabs/favorX/pkg/file/loadsave"
	"github.com/FavorLabs/favorX/pkg/fileinfo"
	"github.com/FavorLabs/favorX/pkg/localstore/filestore"
	"github.com/FavorLabs/favorX/pkg/manifest"
	"github.com/FavorLabs/favorX/pkg/sctx"
	"github.com/FavorLabs/favorX/pkg/storage"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/langos"
	"github.com/gauss-project/aurorafs/pkg/aurora"
	"github.com/gauss-project/aurorafs/pkg/boson"
	"github.com/gauss-project/aurorafs/pkg/jsonhttp"
	"github.com/gauss-project/aurorafs/pkg/tracing"
	"github.com/gorilla/mux"
)

var (
	ErrNotFound    = errors.New("manifest: not found")
	ErrServerError = errors.New("manifest: ServerError")
)

func (s *server) auroraUploadHandler(w http.ResponseWriter, r *http.Request) {
	logger := tracing.NewLoggerWithTraceID(r.Context(), s.logger)

	contentType := r.Header.Get(contentTypeHeader)
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		logger.Debugf("upload: parse content type header %q: %v", contentType, err)
		logger.Errorf("upload: parse content type header %q", contentType)
		jsonhttp.BadRequest(w, invalidContentType)
		return
	}
	isDir := r.Header.Get(AuroraCollectionHeader)
	if strings.ToLower(isDir) == StringTrue || mediaType == multiPartFormData {
		s.dirUploadHandler(w, r)
		return
	}
	s.fileUploadHandler(w, r)
}

// auroraUploadResponse is returned when an HTTP request to upload a file or collection is successful
type auroraUploadResponse struct {
	Reference boson.Address `json:"reference"`
}

type auroraRegisterResponse struct {
	Hash common.Hash `json:"hash"`
}

// fileUploadHandler uploads the file and its metadata supplied in the file body and
// the headers
func (s *server) fileUploadHandler(w http.ResponseWriter, r *http.Request) {
	logger := tracing.NewLoggerWithTraceID(r.Context(), s.logger)
	var (
		reader            io.Reader
		dirName, fileName string
	)

	// Content-Type has already been validated by this time
	contentType := r.Header.Get(contentTypeHeader)

	ctx := r.Context()

	fileName = r.URL.Query().Get("name")
	dirName = r.Header.Get(AuroraCollectionNameHeader)
	reader = r.Body

	p := requestPipelineFn(s.storer, r)

	// first store the file and get its reference
	fr, err := p(ctx, reader)
	if err != nil {
		logger.Debugf("upload file: file len, file %q: %v", fileName, err)
		logger.Errorf("upload file: file len, file %q", fileName)
		jsonhttp.InternalServerError(w, nil)
		return
	}

	bitLen, err := s.fileInfo.GetFileSize(fr)
	if err != nil {
		jsonhttp.InternalServerError(w, fileStoreError)
		return
	}
	// If filename is still empty, use the file hash as the filename
	if fileName == "" {
		fileName = fr.String()
	}

	encrypt := requestEncrypt(r)
	factory := requestPipelineFactory(ctx, s.storer, r)
	l := loadsave.New(s.storer, factory)

	m, err := manifest.NewDefaultManifest(l, encrypt)
	if err != nil {
		logger.Debugf("upload file: create manifest, file %q: %v", fileName, err)
		logger.Errorf("upload file: create manifest, file %q", fileName)
		jsonhttp.InternalServerError(w, nil)
		return
	}

	realIndexFilename, err := UnescapeUnicode(fileName)
	if err != nil {
		logger.Debugf("upload file: filename %q unescape err: %v", fileName, err)
		logger.Errorf("upload file: filename %q unescape err", fileName)
		jsonhttp.BadRequest(w, nil)
		return
	}

	rootMtdt := map[string]string{
		manifest.WebsiteIndexDocumentSuffixKey: realIndexFilename,
	}

	if dirName != "" {
		realDirName, err := UnescapeUnicode(dirName)
		if err != nil {
			logger.Debugf("upload file: dirname %q unescape err: %v", dirName, err)
			logger.Errorf("upload file: dirname %q unescape err", dirName)
			jsonhttp.BadRequest(w, nil)
			return
		}
		rootMtdt[manifest.EntryMetadataDirnameKey] = realDirName
	}

	err = m.Add(ctx, manifest.RootPath, manifest.NewEntry(boson.ZeroAddress, rootMtdt, 0))
	if err != nil {
		logger.Debugf("upload file: adding metadata to manifest, file %q: %v", fileName, err)
		logger.Errorf("upload file: adding metadata to manifest, file %q", fileName)
		jsonhttp.InternalServerError(w, nil)
		return
	}

	fileMtdt := map[string]string{
		manifest.EntryMetadataContentTypeKey: contentType,
		manifest.EntryMetadataFilenameKey:    realIndexFilename,
	}

	err = m.Add(ctx, fileName, manifest.NewEntry(fr, fileMtdt, 0))
	if err != nil {
		logger.Debugf("upload file: adding file to manifest, file %q: %v", fileName, err)
		logger.Errorf("upload file: adding file to manifest, file %q", fileName)
		jsonhttp.InternalServerError(w, nil)
		return
	}

	logger.Debugf("Uploading file Encrypt: %v Filename: %s Filehash: %s FileMtdt: %v",
		encrypt, fileName, fr.String(), fileMtdt)

	var storeSizeFn []manifest.StoreSizeFunc

	manifestReference, err := m.Store(ctx, storeSizeFn...)
	fn := func(reference boson.Address) error {
		bitLen++
		return nil
	}
	err = m.IterateAddresses(ctx, fn)
	if err != nil {
		logger.Debugf("upload file: manifest store, file %q: %v", fileName, err)
		logger.Errorf("upload file: manifest store, file %q", fileName)
		jsonhttp.InternalServerError(w, nil)
		return
	}
	logger.Debugf("Manifest Reference: %s", manifestReference.String())

	err = s.chunkInfo.OnFileUpload(ctx, manifestReference, bitLen)
	if err != nil {
		logger.Debugf("upload file: chunk transfer data err: %v", err)
		logger.Errorf("upload file: chunk transfer data err")
		jsonhttp.InternalServerError(w, "chunk transfer data error")
		return
	}

	if strings.ToLower(r.Header.Get(AuroraPinHeader)) == StringTrue {
		if err := s.pinning.CreatePin(ctx, manifestReference, false); err != nil {
			logger.Debugf("upload file: creation of pin for %q failed: %v", manifestReference, err)
			logger.Error("upload file: creation of pin failed")
			jsonhttp.InternalServerError(w, nil)
			return
		}
		err = s.fileInfo.PinFile(manifestReference, true)
		if err != nil {
			s.logger.Errorf("upload file:update fileinfo pin failed:%v", err)
		}
	}
	err = s.fileInfo.AddFile(manifestReference)
	if err != nil {
		jsonhttp.NotFound(w, "add file error")
		return
	}
	w.Header().Set("ETag", fmt.Sprintf("%q", manifestReference.String()))
	jsonhttp.Created(w, auroraUploadResponse{
		Reference: manifestReference,
	})
}

func (s *server) auroraDownloadHandler(w http.ResponseWriter, r *http.Request) {
	logger := tracing.NewLoggerWithTraceID(r.Context(), s.logger)
	ls := loadsave.NewReadonly(s.storer, storage.ModeGetRequest)

	targets := r.URL.Query().Get("targets")
	if targets != "" {
		r = r.WithContext(sctx.SetTargets(r.Context(), targets))
	}

	nameOrHex := mux.Vars(r)["address"]
	pathVar := mux.Vars(r)["path"]
	if strings.HasSuffix(pathVar, "/") {
		pathVar = strings.TrimRight(pathVar, "/")
		// NOTE: leave one slash if there was some
		pathVar += "/"
	}

	address, err := s.resolveNameOrAddress(nameOrHex)
	if err != nil {
		logger.Debugf("download: parse address %s: %v", nameOrHex, err)
		logger.Error("download: parse address")
		jsonhttp.NotFound(w, nil)
		return
	}

	r = r.WithContext(sctx.SetRootHash(r.Context(), address))
	if !s.chunkInfo.Discover(r.Context(), nil, address) {
		logger.Debugf("download: chunkInfo init %s: %v", nameOrHex, err)
		jsonhttp.NotFound(w, nil)
		return
	}

	ctx := r.Context()

	// read manifest entry
	m, err := manifest.NewDefaultManifestReference(
		address,
		ls,
	)
	if err != nil {
		logger.Debugf("download: not manifest %s: %v", address, err)
		logger.Errorf("download: not manifest %s", address)
		jsonhttp.NotFound(w, nil)
		return
	}
	chunks := 0
	fn := func(nodeType int, path, prefix, hash []byte, metadata map[string]string) error {
		if nodeType == 0 {
			chunks++
		}
		return nil
	}
	err = m.IterateDirectories(ctx, []byte(""), 0, fn)
	if err != nil {
		jsonhttp.NotFound(w, "path address not found")
		return
	}
	if pathVar == "" {
		logger.Debugf("download: handle empty path %s", address)

		if indexDocumentSuffixKey, ok := manifestMetadataLoad(ctx, m, manifest.RootPath, manifest.WebsiteIndexDocumentSuffixKey); ok {
			pathVar = path.Join(pathVar, indexDocumentSuffixKey)
			indexDocumentManifestEntry, err := m.Lookup(ctx, pathVar)
			if err == nil && chunks == 1 {

				// index document exists
				logger.Debugf("download: serving path: %s", pathVar)

				s.serveManifestEntry(w, r, address, indexDocumentManifestEntry, true)
				return
			}
		}
	}

	me, err := m.Lookup(ctx, pathVar)

	if err != nil {
		logger.Debugf("download: invalid path %s/%s: %v", address, pathVar, err)
		logger.Error("download: invalid path")

		if errors.Is(err, manifest.ErrNotFound) {

			if !strings.HasPrefix(pathVar, "/") {
				// check for directory
				dirPath := pathVar + "/"
				exists, err := m.HasPrefix(r.Context(), dirPath)
				if err == nil && exists {
					// redirect to directory
					u := r.URL
					u.Path += "/"
					redirectURL := u.String()

					logger.Debugf("download: redirecting to %s: %v", redirectURL, err)

					http.Redirect(w, r, redirectURL, http.StatusPermanentRedirect)
					return
				}
			}

			// check index suffix path
			if indexDocumentSuffixKey, ok := manifestMetadataLoad(r.Context(), m, manifest.RootPath, manifest.WebsiteIndexDocumentSuffixKey); ok {
				if !strings.HasSuffix(pathVar, indexDocumentSuffixKey) {
					// check if path is directory with index
					pathWithIndex := path.Join(pathVar, indexDocumentSuffixKey)
					indexDocumentManifestEntry, err := m.Lookup(r.Context(), pathWithIndex)
					if err == nil {
						// index document exists
						logger.Debugf("download: serving path: %s", pathWithIndex)

						s.serveManifestEntry(w, r, address, indexDocumentManifestEntry, true)
						return
					}
				}
			}

			// check if error document is to be shown
			if errorDocumentPath, ok := manifestMetadataLoad(r.Context(), m, manifest.RootPath, manifest.WebsiteErrorDocumentPathKey); ok {
				if pathVar != errorDocumentPath {
					errorDocumentManifestEntry, err := m.Lookup(r.Context(), errorDocumentPath)
					if err == nil {
						// error document exists
						logger.Debugf("download: serving path: %s", errorDocumentPath)

						s.serveManifestEntry(w, r, address, errorDocumentManifestEntry, true)
						return
					}
				}
			}

			jsonhttp.NotFound(w, "path address not found")
		} else {
			jsonhttp.NotFound(w, nil)
		}
		return
	}
	me = manifest.NewEntry(me.Reference(), me.Metadata(), 0)

	if !s.chunkInfo.Discover(r.Context(), nil, me.Reference()) {
		logger.Debugf("download: chunkInfo init %s: %v", nameOrHex, err)
		jsonhttp.NotFound(w, nil)
		return
	}
	r = r.WithContext(sctx.SetRootHash(r.Context(), me.Reference()))
	// serve requested path
	s.serveManifestEntry(w, r, address, me, true)
}

func (s *server) serveManifestEntry(
	w http.ResponseWriter,
	r *http.Request,
	rootCid boson.Address,
	manifestEntry manifest.Entry,
	etag bool,
) {
	additionalHeaders := http.Header{}
	metadata := manifestEntry.Metadata()
	if fname, ok := metadata[manifest.EntryMetadataFilenameKey]; ok {
		fname = filepath.Base(fname) // only keep the file name
		additionalHeaders["Content-Disposition"] =
			[]string{fmt.Sprintf("inline; filename=\"%s\"", fname)}
	}

	if mimeType, ok := metadata[manifest.EntryMetadataContentTypeKey]; ok {
		additionalHeaders["Content-Type"] = []string{mimeType}
	}

	s.downloadHandler(w, r, rootCid, manifestEntry.Reference(), manifestEntry.Index(), additionalHeaders, etag)
}

// downloadHandler contains common logic for dowloading file from API
func (s *server) downloadHandler(w http.ResponseWriter, r *http.Request, rootCid, reference boson.Address, index int64, additionalHeaders http.Header, etag bool) {
	logger := tracing.NewLoggerWithTraceID(r.Context(), s.logger)
	targets := r.URL.Query().Get("targets")
	if targets != "" {
		r = r.WithContext(sctx.SetTargets(r.Context(), targets))
	}
	_, _ = s.storer.Get(r.Context(), storage.ModeGetRequest, reference, index)
	length, err := s.fileInfo.GetFileSize(reference)
	if err != nil {
		jsonhttp.BadRequest(w, "path address not found")
		return
	}
	if length > 1 && index == 0 {
		length++
	}

	if index > 0 {
		length += index + 1
	}

	r = r.WithContext(sctx.SetRootLen(r.Context(), length))
	reader, l, err := joiner.New(r.Context(), s.storer, storage.ModeGetRequest, reference, index)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			logger.Debugf("api download: not found %s: %v", reference, err)
			logger.Error("api download: not found")
			jsonhttp.NotFound(w, nil)
			return
		}
		logger.Debugf("api download: unexpected error %s: %v", reference, err)
		logger.Error("api download: unexpected error")
		jsonhttp.InternalServerError(w, nil)
		return
	}
	err = s.fileInfo.AddFile(rootCid)
	if err != nil {
		s.logger.Error(err.Error())
		jsonhttp.BadRequest(w, "add file error")
		return
	}
	// include additional headers
	for name, values := range additionalHeaders {
		w.Header().Set(name, strings.Join(values, "; "))
	}

	if etag {
		w.Header().Set("ETag", fmt.Sprintf("%q", reference))
	}

	// http cache policy
	w.Header().Set("Cache-Control", "no-store")

	w.Header().Set("Content-Length", fmt.Sprintf("%d", l))
	w.Header().Set("Decompressed-Content-Length", fmt.Sprintf("%d", l))
	w.Header().Set("Access-Control-Expose-Headers", "Content-Disposition")
	if targets != "" {
		w.Header().Set(TargetsRecoveryHeader, targets)
	}
	http.ServeContent(w, r, "", time.Now(), langos.NewBufferedLangos(reader, lookaheadBufferSize(l)))
}

type auroraListResponse struct {
	RootCid   boson.Address          `json:"rootCid"`
	PinState  bool                   `json:"pinState"`
	BitVector aurora.BitVectorApi    `json:"bitVector"`
	Register  bool                   `json:"register"`
	Manifest  *fileinfo.ManifestNode `json:"manifest"`
}

type auroraPageResponse struct {
	Total int                  `json:"total"`
	List  []auroraListResponse `json:"list"`
}

func (s *server) auroraListHandler(w http.ResponseWriter, r *http.Request) {
	var reqs aurora.ApiBody
	isBody := true
	isPage := false
	pageTotal := 0

	req, err := ioutil.ReadAll(r.Body)
	if err != nil {
		s.logger.Error("file list: Request parameter acquisition failed,%v", err.Error())
		jsonhttp.InternalServerError(w, fmt.Errorf("file list: Request parameter acquisition failed,%v", err.Error()))
		return
	}

	if len(req) == 0 {
		isBody = false
	}

	page := r.URL.Query().Get("page")
	if page != "" {
		isPage = true
		var apiPage aurora.ApiPage
		err := json.Unmarshal([]byte(page), &apiPage)
		if err != nil {
			s.logger.Error("file list: Request parameter conversion failed,%v", err.Error())
			jsonhttp.InternalServerError(w, fmt.Errorf("file list: Request parameter conversion failed,%v", err.Error()))
			return
		}
		reqs.Page = apiPage
	}
	filter := r.URL.Query().Get("filter")
	if filter != "" {
		isPage = true
		apiFilters := make([]aurora.ApiFilter, 0, 7)
		err := json.Unmarshal([]byte(filter), &apiFilters)
		if err != nil {
			s.logger.Error("file list: Request parameter conversion failed,%v", err.Error())
			jsonhttp.InternalServerError(w, fmt.Errorf("file list: Request parameter conversion failed,%v", err.Error()))
			return
		}
		reqs.Filter = apiFilters
	}
	asort := r.URL.Query().Get("sort")
	if asort != "" {
		isPage = true
		var apiSort aurora.ApiSort
		err := json.Unmarshal([]byte(asort), &apiSort)
		if err != nil {
			s.logger.Error("file list: Request parameter conversion failed,%v", err.Error())
			jsonhttp.InternalServerError(w, fmt.Errorf("file list: Request parameter conversion failed,%v", err.Error()))
			return
		}
		reqs.Sort = apiSort
	}
	if isBody {
		isPage = true
		err = json.Unmarshal(req, &reqs)
		if err != nil {
			s.logger.Error("file list: Request parameter conversion failed,%v", err.Error())
			jsonhttp.InternalServerError(w, fmt.Errorf("file list: Request parameter conversion failed,%v", err.Error()))
			return
		}
	}
	if isPage {
		if reqs.Page.PageSize == reqs.Page.PageNum && reqs.Page.PageSize == 0 {
			jsonhttp.InternalServerError(w, fmt.Errorf("file list: Page information error"))
			return
		}
	}

	var filePage filestore.Page
	var fileFilter []filestore.Filter
	var fileSort filestore.Sort
	if isPage {
		filePage = filestore.Page{
			PageNum:  reqs.Page.PageNum,
			PageSize: reqs.Page.PageSize,
		}
		fileFilter = make([]filestore.Filter, 0, len(reqs.Filter))
		for i := range reqs.Filter {
			fileFilter = append(fileFilter, filestore.Filter{
				Key:   reqs.Filter[i].Key,
				Term:  reqs.Filter[i].Term,
				Value: reqs.Filter[i].Value,
			})
		}
		fileSort = filestore.Sort{
			Key:   reqs.Sort.Key,
			Order: reqs.Sort.Order,
		}

	}
	fileListInfo := s.fileInfo.GetFileList(filePage, fileFilter, fileSort)

	responseList := make([]auroraListResponse, 0)

	for i := range fileListInfo {
		responseList = append(responseList, auroraListResponse{
			RootCid:  fileListInfo[i].RootCid,
			PinState: fileListInfo[i].Pinned,
			BitVector: aurora.BitVectorApi{
				Len: fileListInfo[i].BvLen,
				B:   fileListInfo[i].Bv,
			},
			Register: fileListInfo[i].Registered,
			Manifest: &fileinfo.ManifestNode{
				Type:      fileListInfo[i].Type,
				Hash:      fileListInfo[i].Hash,
				Name:      fileListInfo[i].Name,
				Size:      uint64(fileListInfo[i].Size),
				Extension: fileListInfo[i].Extension,
				Default:   fileListInfo[i].Default,
				MimeType:  fileListInfo[i].MimeType,
			},
		})
	}
	if !isPage {
		zeroAddress := boson.NewAddress([]byte{31: 0})
		sort.Slice(responseList, func(i, j int) bool {
			closer, _ := responseList[i].RootCid.Closer(zeroAddress, responseList[j].RootCid)
			return closer
		})
		jsonhttp.OK(w, responseList)
	} else {
		pageResponseList := auroraPageResponse{
			Total: pageTotal,
			List:  responseList,
		}
		jsonhttp.OK(w, pageResponseList)
	}

}

// manifestMetadataLoad returns the value for a key stored in the metadata of
// manifest path, or empty string if no value is present.
// The ok result indicates whether value was found in the metadata.
func manifestMetadataLoad(
	ctx context.Context,
	manifest manifest.Interface,
	path, metadataKey string,
) (string, bool) {
	me, err := manifest.Lookup(ctx, path)
	if err != nil {
		return "", false
	}

	manifestRootMetadata := me.Metadata()
	if val, ok := manifestRootMetadata[metadataKey]; ok {
		return val, ok
	}

	return "", false
}

// manifestViewHandler
func (s *server) manifestViewHandler(w http.ResponseWriter, r *http.Request) {

	nameOrHex := mux.Vars(r)["address"]
	pathVar := mux.Vars(r)["path"]

	depth := 1
	if r.URL.Query().Get("recursive") != "" {
		depth = -1
	}

	rootNode, err := s.fileInfo.ManifestView(r.Context(), nameOrHex, pathVar, depth)
	if errors.Is(err, ErrNotFound) {
		jsonhttp.NotFound(w, nil)
	}
	if errors.Is(err, ErrServerError) {
		jsonhttp.InternalServerError(w, nil)
	}

	jsonhttp.OK(w, rootNode)
}

func (s *server) fileRegister(w http.ResponseWriter, r *http.Request) {
	apiName := "fileRegister"
	logger := tracing.NewLoggerWithTraceID(r.Context(), s.logger)
	nameOrHex := mux.Vars(r)["address"]
	address, err := s.resolveNameOrAddress(nameOrHex)
	defer s.tranProcess.Delete(apiName + address.String())

	if err != nil {
		logger.Debugf("file fileRegister: parse address %s: %v", nameOrHex, err)
		logger.Errorf("file fileRegister: parse address")
		jsonhttp.NotFound(w, nil)
		return
	}
	if _, ok := s.tranProcess.Load(apiName + address.String()); ok {
		logger.Errorf("parse address %s under processing", nameOrHex)
		jsonhttp.InternalServerError(w, fmt.Sprintf("parse address %s under processing", nameOrHex))
		return
	}
	s.tranProcess.Store(apiName+address.String(), "-")
	overlays := s.oracleChain.GetNodesFromCid(address.Bytes())
	for _, v := range overlays {
		if s.overlay.Equal(v) {
			jsonhttp.Forbidden(w, fmt.Sprintf("address:%v Already Register", address.String()))
			return
		}
	}

	hash, err := s.oracleChain.RegisterCidAndNode(r.Context(), address, s.overlay)
	trans := TransactionResponse{
		Hash:     hash,
		Address:  address,
		Register: true,
	}
	s.transactionChan <- trans
	if err != nil {
		logger.Errorf("fileRegister failed: %v ", err)
		jsonhttp.InternalServerError(w, fmt.Sprintf("fileRegister failed: %v ", err))
		return
	}

	jsonhttp.OK(w,
		auroraRegisterResponse{
			Hash: hash,
		})
}

func (s *server) fileRegisterRemove(w http.ResponseWriter, r *http.Request) {
	apiName := "fileRegisterRemove"
	logger := tracing.NewLoggerWithTraceID(r.Context(), s.logger)
	nameOrHex := mux.Vars(r)["address"]
	address, err := s.resolveNameOrAddress(nameOrHex)
	if err != nil {
		logger.Errorf("fileRegisterRemove: parse address")
		jsonhttp.NotFound(w, nil)
		return
	}
	defer s.tranProcess.Delete(apiName + address.String())
	if _, ok := s.tranProcess.Load(apiName + address.String()); ok {
		logger.Errorf("parse address %s under processing", nameOrHex)
		jsonhttp.InternalServerError(w, fmt.Sprintf("parse address %s under processing", nameOrHex))
		return
	}
	s.tranProcess.Store(apiName+address.String(), "-")

	overlays := s.oracleChain.GetNodesFromCid(address.Bytes())
	isDel := false
	for _, v := range overlays {
		if s.overlay.Equal(v) {
			isDel = true
			break
		}
	}
	if !isDel {
		jsonhttp.Forbidden(w, fmt.Sprintf("address:%v Already Remove", address.String()))
	}

	hash, err := s.oracleChain.RemoveCidAndNode(r.Context(), address, s.overlay)
	if err != nil {
		s.logger.Error("fileRegisterRemove failed: %v ", err)
		jsonhttp.InternalServerError(w, fmt.Sprintf("fileRegisterRemove failed: %v ", err))
		return
	}

	err = s.fileInfo.RegisterFile(address, false)
	if err != nil {
		logger.Errorf("fileRegister update info:%v", err)
	}

	trans := TransactionResponse{
		Hash:     hash,
		Address:  address,
		Register: false,
	}
	s.transactionChan <- trans

	jsonhttp.OK(w,
		auroraRegisterResponse{
			Hash: hash,
		})
}
