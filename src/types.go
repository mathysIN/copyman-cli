package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"strconv"
)

// Types for API responses and data models

type Timestamp int64

func (t *Timestamp) UnmarshalJSON(b []byte) error {
	var raw interface{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}

	switch v := raw.(type) {
	case float64:
		*t = Timestamp(int64(v))
	case string:
		ms, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid timestamp format: %v", v)
		}
		*t = Timestamp(ms)
	default:
		return fmt.Errorf("invalid timestamp format: %v", raw)
	}
	return nil
}

type GetSessionResponseBody struct {
	SessionID        string `json:"sessionId"`
	Password         string `json:"password"`
	CreateNewSession bool   `json:"createNewSession"`
	HasPassword      bool   `json:"hasPassword"`
	IsValidPassword  bool   `json:"isValidPassword"`
	CreatedAt        string `json:"createdAt"`
	SessionToken     string `json:"sessionToken"`
}

type SessionCheckResponse struct {
	Valid            bool   `json:"valid"`
	HasPassword      bool   `json:"hasPassword"`
	IsEncrypted      bool   `json:"isEncrypted"`
	CreateNewSession bool   `json:"createNewSession"`
	CreatedAt        string `json:"createdAt"`
}

type PostNoteRequestBody struct {
	Content string `json:"content"`
}

type KeyValue struct {
	Key   string
	Value string
}

type Config struct {
	SessionID    string
	SessionToken string
	CreatedAt    string
	EncKey       string
}

type BaseContentType struct {
	ID        string    `json:"id"`
	CreatedAt Timestamp `json:"createdAt"`
	UpdatedAt Timestamp `json:"updatedAt"`
	Type      string    `json:"type"`
}

type NoteType struct {
	BaseContentType
	Content       string `json:"content"`
	IsEncrypted   bool   `json:"isEncrypted,omitempty"`
	EncryptedIv   string `json:"encryptedIv,omitempty"`
	EncryptedSalt string `json:"encryptedSalt,omitempty"`
}

type AttachmentType struct {
	BaseContentType
	AttachmentURL  string `json:"attachmentURL"`
	AttachmentPath string `json:"attachmentPath"`
	FileKey        string `json:"fileKey"`
	IsEncrypted    bool   `json:"isEncrypted,omitempty"`
	EncryptedIv    string `json:"encryptedIv,omitempty"`
	EncryptedSalt  string `json:"encryptedSalt,omitempty"`
}

type ContentOutput struct {
	ID             string `json:"id"`
	Type           string `json:"type"`
	Content        string `json:"content,omitempty"`
	AttachmentURL  string `json:"attachmentUrl,omitempty"`
	AttachmentPath string `json:"attachmentPath,omitempty"`
	CreatedAt      int64  `json:"createdAt"`
	UpdatedAt      int64  `json:"updatedAt"`
	IsEncrypted    bool   `json:"isEncrypted,omitempty"`
}

// ContentType interface for polymorphic content
type ContentType interface {
	GetID() string
	GetType() string
}

func (n NoteType) GetID() string         { return n.ID }
func (n NoteType) GetType() string       { return n.Type }
func (a AttachmentType) GetID() string   { return a.ID }
func (a AttachmentType) GetType() string { return a.Type }

// HTTP Client (global for cookie jar)
var jar, _ = cookiejar.New(nil)
var client = http.Client{Jar: jar}
