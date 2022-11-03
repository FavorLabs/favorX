package acl_test

import (
	"testing"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/chain"
	"github.com/centrifuge/go-substrate-rpc-client/v4/signature"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types/codec"
	"github.com/stretchr/testify/assert"
	"github.com/vedhavyas/go-subkey"
)

const url = "ws://127.0.0.1:9944"

func TestAcl_PublicKey(t *testing.T) {
	a := signature.TestKeyringPairAlice.PublicKey

	assert.Equal(t, "0xd43593c715fdd31c61141abd04a99fd6822c8558854ccde39a5684e7a56da27d", codec.HexEncodeToString(a))
	address, err := subkey.SS58Address(a, 42)
	assert.NoError(t, err)
	assert.Equal(t, "5GrwvaEF5zXb26Fz9rcQpDWS57CtERHpNehXCPcNoHGKutQY", address)
}

func TestAcl_SelfNickname(t *testing.T) {
	cli, err := chain.NewClient(url, signature.TestKeyringPairAlice)
	assert.NoError(t, err)

	err = cli.Acl.SetNicknameWatch("bababa")
	assert.NoError(t, err)

	name, err := cli.Acl.GetNickName("0xd43593c715fdd31c61141abd04a99fd6822c8558854ccde39a5684e7a56da27d")
	assert.NoError(t, err)
	assert.Equal(t, "bababa", name)
	name1, err1 := cli.Acl.GetNickName("d43593c715fdd31c61141abd04a99fd6822c8558854ccde39a5684e7a56da27d")
	assert.NoError(t, err1)
	assert.Equal(t, "bababa", name1)

	id, err := cli.Acl.GetAccountID("bababa")
	assert.NoError(t, err)
	assert.Equal(t, "0xd43593c715fdd31c61141abd04a99fd6822c8558854ccde39a5684e7a56da27d", id)
}

func TestAcl_Resolve(t *testing.T) {
	cli, err := chain.NewClient(url, signature.TestKeyringPairAlice)
	assert.NoError(t, err)

	err = cli.Acl.SetResolveWatch("bababa/kk", codec.MustHexDecodeString("0xfe9799739b3d9677972a3b58ef609ba78332428f85ed2534d0b496108a4d92ae"))
	assert.NoError(t, err)

	findCid, err := cli.Acl.GetResolve("/kk")
	assert.NoError(t, err)
	assert.Equal(t, "fe9799739b3d9677972a3b58ef609ba78332428f85ed2534d0b496108a4d92ae", boson.NewAddress(findCid).String())

	findCid2, err := cli.Acl.GetResolve("bababa/kk")
	assert.NoError(t, err)
	assert.Equal(t, "fe9799739b3d9677972a3b58ef609ba78332428f85ed2534d0b496108a4d92ae", boson.NewAddress(findCid2).String())
}
