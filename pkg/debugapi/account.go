package debugapi

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"path/filepath"

	"github.com/FavorLabs/favorX/pkg/jsonhttp"
	"github.com/FavorLabs/favorX/pkg/keystore/subkey"
	"github.com/gogf/gf/v2/encoding/gjson"
)

func (s *Service) importKeyHandler(w http.ResponseWriter, r *http.Request) {
	path := filepath.Join(s.nodeOptions.DataDir, "keys")
	f := subkey.New(path)
	req, err := ioutil.ReadAll(r.Body)
	if err != nil {
		jsonhttp.InternalServerError(w, err)
		return
	}
	j, err := gjson.DecodeToJson(req)
	if err != nil {
		jsonhttp.InternalServerError(w, err)
		return
	}
	password := j.Get("password").String()
	passwordNow := j.Get("password_now").String()
	keyJson := j.Get("keystore").String()
	pkData := j.Get("private_key").String()
	if pkData == "" && keyJson == "" {
		jsonhttp.InternalServerError(w, "Please enter the private_key or keystore")
		return
	}
	if keyJson != "" {
		err := f.ImportKey("boson", password, passwordNow, []byte(keyJson))
		if err != nil {
			jsonhttp.InternalServerError(w, err)
			return
		}
	} else {
		err = f.ImportPrivateKey("boson", password, passwordNow, pkData)
		if err != nil {
			jsonhttp.InternalServerError(w, err)
			return
		}
	}

	jsonhttp.OK(w, nil)
}

func (s *Service) exportKeyHandler(w http.ResponseWriter, r *http.Request) {
	path := filepath.Join(s.nodeOptions.DataDir, "keys")
	f := subkey.New(path)
	req, err := ioutil.ReadAll(r.Body)
	if err != nil {
		jsonhttp.InternalServerError(w, err)
		return
	}
	j, err := gjson.DecodeToJson(req)
	if err != nil {
		jsonhttp.InternalServerError(w, err)
		return
	}
	password := j.Get("password").String()
	tp := j.Get("type").String()
	if tp == "private" {
		kp, _, err := f.Key("boson", password)
		if err != nil {
			jsonhttp.InternalServerError(w, err)
			return
		}
		type out struct {
			PrivateKey string `json:"private_key"`
		}
		var data string
		if kp.GetMnemonic() != "" {
			data = kp.GetMnemonic()
		} else if kp.GetSeed() != nil {
			data = fmt.Sprintf("0x%x", kp.GetSeed())
		} else {
			jsonhttp.InternalServerError(w, errors.New("please export keystore json"))
			return
		}
		jsonhttp.OK(w, out{PrivateKey: data})
		return
	}
	b, err := f.ExportKey("boson", password)
	if err != nil {
		jsonhttp.InternalServerError(w, err)
		return
	}
	w.Header().Set("Content-Type", jsonhttp.DefaultContentTypeHeader)
	_, _ = io.Copy(w, bytes.NewBuffer(b))
}
