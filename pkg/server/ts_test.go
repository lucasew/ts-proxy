package server

import (
	"testing"

	"tailscale.com/tsnet"
)

func TestCurrentTSClearKeepsReplacement(t *testing.T) {
	var cur currentTS
	first := &tsnet.Server{}
	second := &tsnet.Server{}

	cur.set(first)
	if cur.get() != first {
		t.Fatal("get after set")
	}
	cur.set(second)
	cur.clear(first)
	if cur.get() != second {
		t.Fatal("clear of the old node dropped the replacement")
	}
	cur.clear(second)
	if cur.get() != nil {
		t.Fatal("clear of the current node left it published")
	}
}

func TestCurrentTSTakeRemovesNode(t *testing.T) {
	var cur currentTS
	node := &tsnet.Server{}
	cur.set(node)
	if cur.take() != node {
		t.Fatal("take")
	}
	if cur.get() != nil {
		t.Fatal("get after take")
	}
	if cur.take() != nil {
		t.Fatal("take of an empty slot")
	}
}
