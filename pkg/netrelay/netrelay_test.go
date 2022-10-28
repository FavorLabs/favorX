package netrelay

import (
	"strings"
	"testing"

	"github.com/FavorLabs/favorX/pkg/address"
)

func TestService_RelayHttpDo(t *testing.T) {
	url := strings.ReplaceAll(address.RelayPrefixHttp+"/test1/test2", address.RelayPrefixHttp, "")
	urls := strings.Split(url, "/")
	group := urls[1]
	if group != "test1" {
		t.Fatal("url parse err")
	}
}
