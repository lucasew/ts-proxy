package server

import (
	"testing"

	"github.com/stretchr/testify/require"
	"tailscale.com/tsnet"
)

func TestCurrentTSClearKeepsReplacement(t *testing.T) {
	var cur currentTS
	first := &tsnet.Server{}
	second := &tsnet.Server{}

	cur.set(first)
	require.Same(t, first, cur.get())
	cur.set(second)
	cur.clear(first)
	require.Same(t, second, cur.get())
	cur.clear(second)
	require.Nil(t, cur.get())
}

func TestCurrentTSTakeRemovesNode(t *testing.T) {
	var cur currentTS
	node := &tsnet.Server{}
	cur.set(node)
	require.Same(t, node, cur.take())
	require.Nil(t, cur.get())
	require.Nil(t, cur.take())
}
