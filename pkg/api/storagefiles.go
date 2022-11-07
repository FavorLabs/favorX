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
	FileSize uint64        `json:"fileSize"`
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

	users, err := s.commonChain.Storage.PlaceOrderWatch(r.Context(), req.Cid.Bytes(), req.FileSize, req.FileCopy, req.Expire)
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
