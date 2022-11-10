package debugapi

import (
	"bytes"
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
	keyJson := j.Get("keystore").String()
	pkData := j.Get("private_key").String()
	if pkData == "" && keyJson == "" {
		jsonhttp.InternalServerError(w, "Please enter the private_key or keystore")
		return
	}
	if keyJson != "" {
		err := f.ImportKey("boson", password, []byte(keyJson))
		if err != nil {
			jsonhttp.InternalServerError(w, err)
			return
		}
	} else {
		err = f.ImportPrivateKey("boson", password, pkData)
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
		var data = func() string {
			if kp.GetMnemonic() == "" {
				return fmt.Sprintf("0x%x", kp.GetSeed())
			}
			return kp.GetMnemonic()
		}
		jsonhttp.OK(w, out{PrivateKey: data()})
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
