package storagefiles

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/cavaliergopher/grab/v3"
)

const (
	DownloadBufferSize = 256 * 1024
)

var DefaultTransport = &http.Transport{
	Proxy: http.ProxyFromEnvironment,
	DialContext: (&net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext,
	ForceAttemptHTTP2:     true,
	MaxIdleConns:          100,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
	TLSClientConfig: &tls.Config{
		InsecureSkipVerify: true,
	},
}

func DownloadFile(ctx context.Context, gateway, hash, source string, header http.Header) (*grab.Response, error) {
	urlStr := fmt.Sprintf("%s%s/%s?targets=%s", gateway, "/file", hash, source)
	req, err := grab.NewRequest(".", urlStr)
	if err != nil {
		return nil, err
	}

	req.WithContext(ctx)
	req.NoStore = true
	req.BufferSize = DownloadBufferSize
	req.HTTPRequest.Header = header

	client := &grab.Client{
		HTTPClient: &http.Client{Transport: DefaultTransport},
		UserAgent:  "grab",
	}
	resp := client.Do(req)

	return resp, nil
}

func GetSize(ctx context.Context, gateway, hash, source string) (int64, error) {
	urlStr := fmt.Sprintf("%s%s/%s?targets=%s", gateway, "/file", hash, source)
	req, err := http.NewRequest(http.MethodGet, urlStr, nil)
	if err != nil {
		return 0, err
	}

	req.WithContext(ctx)
	req.Header = map[string][]string{
		"Cache-Control": {"no-cache"},
		"Range":         {"bytes=0-1"},
	}
	client := &http.Client{Transport: DefaultTransport}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}

	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("head to file: %s", resp.Status)
	}

	var val int64

	valStr := resp.Header.Get("Decompressed-Content-Length")

	if valStr == "" {
		return 0, fmt.Errorf("content length is zero")
	}

	val, err = strconv.ParseInt(valStr, 10, 64)
	if err != nil {
		return 0, err
	}

	return val, nil
}

func PinFile(ctx context.Context, gateway, hash string) error {
	urlStr := fmt.Sprintf("%s%s/%s", gateway, "/pins", hash)
	req, err := http.NewRequest(http.MethodPost, urlStr, nil)
	if err != nil {
		return err
	}

	req.WithContext(ctx)

	client := &http.Client{Transport: DefaultTransport}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("failed to pin file %s, http %s", hash, resp.Status)
	}

	return nil
}
