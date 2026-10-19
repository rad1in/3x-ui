package model

// Passkey is one WebAuthn credential an admin registered. The public key is the
// COSE_Key as the authenticator sent it; CredentialId is unpadded base64url.
type Passkey struct {
	Id           int    `json:"id" gorm:"primaryKey;autoIncrement" example:"1"`
	UserId       int    `json:"userId" gorm:"index;not null" example:"1"`
	Name         string `json:"name" gorm:"not null;default:''" example:"YubiKey"`
	CredentialId string `json:"-" gorm:"uniqueIndex;size:512;not null"`
	PublicKey    []byte `json:"-"`
	SignCount    int64  `json:"-" gorm:"not null;default:0"`
	CreatedAt    int64  `json:"createdAt" gorm:"not null" example:"1760684400000"`
	LastUsedAt   int64  `json:"lastUsedAt" gorm:"not null;default:0" example:"1760684400000"`
}

func (Passkey) TableName() string { return "passkeys" }
