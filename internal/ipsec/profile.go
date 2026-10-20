package ipsec

import (
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

func profileUUID(inst Instance, user, part string) string {
	return strings.ToUpper(uuid.NewSHA1(uuid.NameSpaceURL, []byte(fmt.Sprintf("x-ui-ikev2:%s:%d:%s:%s", inst.Address, inst.Id, user, part))).String())
}

func xmlText(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// MobileConfig is an Apple configuration profile (iOS, iPadOS, macOS): the
// VPN with its login filled in, plus the CA the server cert chains to.
func MobileConfig(inst Instance, user, password, remark string) []byte {
	ca, err := CACert(inst.PKI)
	var caData string
	if err == nil {
		caData = base64.StdEncoding.EncodeToString(ca.Raw)
	}
	name := xmlText(remark)
	addr := xmlText(inst.Address)
	dial := xmlText(inst.dialAddress())
	// DH 14 / AES-256 / SHA2-256 is in the server's proposals; iOS needs it named.
	sa := `<dict>
        <key>EncryptionAlgorithm</key><string>AES-256</string>
        <key>IntegrityAlgorithm</key><string>SHA2-256</string>
        <key>DiffieHellmanGroup</key><integer>14</integer>
      </dict>`
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>PayloadContent</key>
  <array>
    <dict>
      <key>IKEv2</key>
      <dict>
        <key>AuthenticationMethod</key><string>None</string>
        <key>ExtendedAuthEnabled</key><integer>1</integer>
        <key>AuthName</key><string>` + xmlText(user) + `</string>
        <key>AuthPassword</key><string>` + xmlText(password) + `</string>
        <key>RemoteAddress</key><string>` + dial + `</string>
        <key>RemoteIdentifier</key><string>` + addr + `</string>
        <key>LocalIdentifier</key><string>` + xmlText(user) + `</string>
        <key>ServerCertificateIssuerCommonName</key><string>` + xmlText(ca.Subject.CommonName) + `</string>
        <key>ServerCertificateCommonName</key><string>` + addr + `</string>
        <key>DeadPeerDetectionRate</key><string>Medium</string>
        <key>IKESecurityAssociationParameters</key>
      ` + sa + `
        <key>ChildSecurityAssociationParameters</key>
      ` + sa + `
      </dict>
      <key>IPv4</key>
      <dict><key>OverridePrimary</key><integer>1</integer></dict>
      <key>PayloadDisplayName</key><string>` + name + `</string>
      <key>PayloadIdentifier</key><string>com.x-ui.ikev2.` + profileUUID(inst, user, "vpn") + `</string>
      <key>PayloadType</key><string>com.apple.vpn.managed</string>
      <key>PayloadUUID</key><string>` + profileUUID(inst, user, "vpn") + `</string>
      <key>PayloadVersion</key><integer>1</integer>
      <key>UserDefinedName</key><string>` + name + `</string>
      <key>VPNType</key><string>IKEv2</string>
    </dict>
    <dict>
      <key>PayloadCertificateFileName</key><string>ca.cer</string>
      <key>PayloadContent</key><data>` + caData + `</data>
      <key>PayloadDisplayName</key><string>` + xmlText(ca.Subject.CommonName) + `</string>
      <key>PayloadIdentifier</key><string>com.x-ui.ikev2.` + profileUUID(inst, user, "ca") + `</string>
      <key>PayloadType</key><string>com.apple.security.root</string>
      <key>PayloadUUID</key><string>` + profileUUID(inst, user, "ca") + `</string>
      <key>PayloadVersion</key><integer>1</integer>
    </dict>
  </array>
  <key>PayloadDisplayName</key><string>` + name + `</string>
  <key>PayloadIdentifier</key><string>com.x-ui.ikev2.` + profileUUID(inst, user, "profile") + `</string>
  <key>PayloadType</key><string>Configuration</string>
  <key>PayloadUUID</key><string>` + profileUUID(inst, user, "profile") + `</string>
  <key>PayloadVersion</key><integer>1</integer>
</dict>
</plist>
`)
}

// SSWAN is a profile for the strongSwan Android app; it asks for the password.
func SSWAN(inst Instance, user, remark string) []byte {
	var cert string
	if ca, err := CACert(inst.PKI); err == nil {
		cert = base64.StdEncoding.EncodeToString(ca.Raw)
	}
	body, _ := json.MarshalIndent(map[string]any{
		"uuid": strings.ToLower(profileUUID(inst, user, "sswan")),
		"name": remark,
		"type": "ikev2-eap",
		"remote": map[string]any{
			"addr": inst.dialAddress(),
			"id":   inst.Address,
			"cert": cert,
		},
		"local": map[string]any{"eap_id": user},
	}, "", "  ")
	return body
}

// CADER is the CA certificate for clients set up by hand, such as Windows.
func CADER(inst Instance) []byte {
	ca, err := CACert(inst.PKI)
	if err != nil {
		return nil
	}
	return ca.Raw
}
