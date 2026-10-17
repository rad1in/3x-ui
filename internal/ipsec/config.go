package ipsec

import (
	"crypto/sha256"
	"encoding/hex"
	"net/netip"
	"sort"
	"strconv"
	"strings"

	"github.com/strongswan/govici/vici"
)

// Windows' built-in client only offers MODP1024 until told otherwise, and
// older Androids only SHA1; "default" keeps every modern suite first.
var (
	ikeProposals = []string{"default", "aes256-sha256-modp2048", "aes256-sha1-modp2048", "aes128-sha1-modp2048", "aes256-sha1-modp1024", "aes128-sha1-modp1024"}
	espProposals = []string{"default", "aes256-sha256", "aes256-sha1", "aes128-sha1"}
)

// User is one account an IKEv2 inbound lets in with EAP-MSCHAPv2.
type User struct {
	Password string
	MaxConns int
}

// Instance is the desired state of one IKEv2 inbound. Every inbound is a
// connection of the one host-wide charon, which owns UDP 500 and 4500.
type Instance struct {
	Id      int
	Tag     string
	Address string // what clients dial; the server cert names it
	Pool    netip.Prefix
	DNS     []string
	PKI     PKI
	Users   map[string]User
}

func connName(id int) string { return "ikev2-" + strconv.Itoa(id) }

// poolName changes with the pool's contents: charon will not redefine a pool
// with leases out, so a new one is loaded and the old dropped once it drains.
func poolName(inst Instance) string {
	sum := sha256.Sum256([]byte(inst.Pool.String() + "|" + strings.Join(inst.DNS, ",")))
	return connName(inst.Id) + "-" + hex.EncodeToString(sum[:4])
}

func eapSecretID(email string) string { return "eap-" + email }

func connMessage(inst Instance) *vici.Message {
	local := vici.NewMessage()
	_ = local.Set("auth", "pubkey")
	_ = local.Set("certs", []string{inst.PKI.ServerCert})
	_ = local.Set("id", inst.Address)

	remote := vici.NewMessage()
	_ = remote.Set("auth", "eap-mschapv2")
	_ = remote.Set("eap_id", "%any")

	child := vici.NewMessage()
	_ = child.Set("local_ts", []string{"0.0.0.0/0"})
	_ = child.Set("esp_proposals", espProposals)
	_ = child.Set("dpd_action", "clear")
	children := vici.NewMessage()
	_ = children.Set("net", child)

	conn := vici.NewMessage()
	_ = conn.Set("version", "2")
	_ = conn.Set("proposals", ikeProposals)
	_ = conn.Set("pools", []string{poolName(inst)})
	_ = conn.Set("send_certreq", "no")
	// Logins are counted per EAP identity by the panel, not per IKE identity.
	_ = conn.Set("unique", "never")
	_ = conn.Set("fragmentation", "yes")
	_ = conn.Set("dpd_delay", "30s")
	_ = conn.Set("local", local)
	_ = conn.Set("remote", remote)
	_ = conn.Set("children", children)
	return conn
}

func poolMessage(inst Instance) *vici.Message {
	pool := vici.NewMessage()
	_ = pool.Set("addrs", inst.Pool.Masked().String())
	if len(inst.DNS) > 0 {
		_ = pool.Set("dns", inst.DNS)
	}
	return pool
}

// desired is everything the daemon should hold, keyed for diffing against
// what was loaded last.
type desired struct {
	conns  map[string]*vici.Message
	pools  map[string]*vici.Message
	shared map[string]sharedSecret
	keys   []string // private keys, PEM
	byConn map[string]Instance
}

type sharedSecret struct {
	Type   string
	Data   string
	Owners []string
}

func (s sharedSecret) fingerprint() string {
	return s.Type + "|" + s.Data + "|" + strings.Join(s.Owners, ",")
}

func desiredOf(insts []Instance) desired {
	d := desired{
		conns:  map[string]*vici.Message{},
		pools:  map[string]*vici.Message{},
		shared: map[string]sharedSecret{},
		byConn: map[string]Instance{},
	}
	for _, inst := range insts {
		name := connName(inst.Id)
		d.conns[name] = connMessage(inst)
		d.pools[poolName(inst)] = poolMessage(inst)
		d.byConn[name] = inst
		d.keys = append(d.keys, inst.PKI.ServerKey)
		for email, u := range inst.Users {
			d.shared[eapSecretID(email)] = sharedSecret{Type: "EAP", Data: u.Password, Owners: []string{email}}
		}
	}
	sort.Strings(d.keys)
	return d
}
