package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/jsonhttp"
)

type RequestPlaceOrder struct {
	Cid      boson.Address `json:"cid"`
	FileCopy uint64        `json:"fileCopy"`
	Expire   uint32        `json:"expire"`
}

func (s *server) placeOrder(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		if jsonhttp.HandleBodyReadError(err, w) {
			return
		}
		s.logger.Error("placeOrder: read req error")
		jsonhttp.InternalServerError(w, "cannot read data")
		return
	}
	var req RequestPlaceOrder
	if err = json.Unmarshal(body, &req); err != nil {
		s.logger.Error("api: placeOrder: unmarshal request body")
		jsonhttp.BadRequest(w, "Unmarshal json body")
		return
	}

	info, err := s.fileInfo.GetFileView(req.Cid)
	if err != nil {
		jsonhttp.InternalServerError(w, err)
		return
	}

	users, err := s.commonChain.Storage.PlaceOrderWatch(r.Context(), req.Cid.Bytes(), uint64(info.Size), req.FileCopy, req.Expire)
	if err != nil {
		jsonhttp.InternalServerError(w, err)
		return
	}
	var res []boson.Address
	for _, v := range users {
		res = append(res, boson.NewAddress(v.ToBytes()))
	}
	jsonhttp.OK(w, res)
}

func (s *server) merchantRegister(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		if jsonhttp.HandleBodyReadError(err, w) {
			return
		}
		s.logger.Error("merchantRegister: read req error")
		jsonhttp.InternalServerError(w, "cannot read data")
		return
	}
	var req struct {
		DiskTotal uint64 `json:"diskTotal"`
	}
	if err = json.Unmarshal(body, &req); err != nil {
		s.logger.Error("api: merchantRegister: unmarshal request body")
		jsonhttp.BadRequest(w, "Unmarshal json body")
		return
	}

	err = s.commonChain.Storage.MerchantRegisterWatch(r.Context(), req.DiskTotal)
	if err != nil {
		jsonhttp.InternalServerError(w, err)
		return
	}
	jsonhttp.OK(w, nil)
}

func (s *server) merchantUnRegister(w http.ResponseWriter, r *http.Request) {
	err := s.commonChain.Storage.MerchantUnregisterWatch(r.Context())
	if err != nil {
		jsonhttp.InternalServerError(w, err)
		return
	}
	jsonhttp.OK(w, nil)
}
