package ipsec

import (
	"reflect"
	"testing"
)

func trafficOf(ts []Traffic) map[string][2]int64 {
	out := map[string][2]int64{}
	for _, t := range ts {
		out[t.Tag+"/"+t.Email] = [2]int64{t.Up, t.Down}
	}
	return out
}

func TestAccountingBooksEachByteOnce(t *testing.T) {
	insts := map[string]Instance{"ikev2-1": {Id: 1, Tag: "in-500-udp"}}
	var a accounting
	a.reset()
	poll := func(in, out int64) map[string][2]int64 {
		sas := []ikeSA{{conn: "ikev2-1", id: 1, user: "ali", children: []childSA{{id: 7, in: in, out: out}}}}
		return trafficOf(a.drain(sas, insts))
	}

	if got, want := poll(100, 200), map[string][2]int64{"in-500-udp/ali": {100, 200}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("first poll = %v, want %v", got, want)
	}
	if got, want := poll(150, 260), map[string][2]int64{"in-500-udp/ali": {50, 60}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("second poll = %v, want the growth %v", got, want)
	}
	// The SA goes down after 20 more bytes; charon may still list it once.
	a.finish("ikev2-1", "ali", childSA{id: 7, in: 170, out: 260})
	if got, want := poll(170, 260), map[string][2]int64{"in-500-udp/ali": {20, 0}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("after teardown = %v, want only the last %v", got, want)
	}
	if got := poll(170, 260); len(got) != 0 {
		t.Fatalf("a finished SA was booked again: %v", got)
	}
}

// A short session can start and end between two polls; its event is all there is.
func TestAccountingKeepsSessionsThatEndBetweenPolls(t *testing.T) {
	insts := map[string]Instance{"ikev2-1": {Id: 1, Tag: "in-500-udp"}}
	var a accounting
	a.reset()
	a.drain(nil, insts)
	a.finish("ikev2-1", "ali", childSA{id: 9, in: 300, out: 4000})
	got := trafficOf(a.drain([]ikeSA{}, insts))
	if want := map[string][2]int64{"in-500-udp/ali": {300, 4000}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestOverLimitClosesTheNewestSessions(t *testing.T) {
	insts := map[string]Instance{"ikev2-1": {Users: map[string]User{
		"one": {MaxConns: 1}, "two": {MaxConns: 2}, "free": {},
	}}}
	sas := []ikeSA{
		{conn: "ikev2-1", id: 1, user: "one", established: 600},
		{conn: "ikev2-1", id: 2, user: "one", established: 30},
		{conn: "ikev2-1", id: 3, user: "one", established: 5},
		{conn: "ikev2-1", id: 4, user: "two", established: 50},
		{conn: "ikev2-1", id: 5, user: "two", established: 40},
		{conn: "ikev2-1", id: 6, user: "free", established: 40},
		{conn: "ikev2-1", id: 7, user: "free", established: 30},
	}
	if got, want := overLimit(sas, insts), []uint64{2, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("overLimit = %v, want %v (the first session keeps its place)", got, want)
	}
}

func TestUserSessionIDsPicksOnlyThatUsersIKEv2Sessions(t *testing.T) {
	sas := []ikeSA{
		{conn: "ikev2-1", id: 1, user: "alice"},
		{conn: "ikev2-1", id: 2, user: "bob"},
		{conn: "ikev2-2", id: 3, user: "alice"},
		{conn: "l2tp-1", id: 4, user: "alice"},
	}
	if got, want := userSessionIDs(sas, "alice"), []uint64{1, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("userSessionIDs = %v, want %v (the L2TP IPsec leg is not an IKEv2 login)", got, want)
	}
}
