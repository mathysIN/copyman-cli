package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/manifoldco/promptui"
	"github.com/mathysin/copyman-cli/utils"
)

const configFolderPath = "/copyman/"
const configFolderFileName = "config"

func createSession(sessionID string, password string, temporary bool) (Config, error) {
	apiURL := utils.API_LINK + utils.API_PATH_SESSION

	// Generate timestamp for key derivation (consistent with web app)
	createdAt := fmt.Sprintf("%d", getTimestamp())

	// Prepare request body
	requestBody := map[string]interface{}{
		"session":   strings.ToLower(sessionID),
		"createdAt": createdAt,
		"join":      false,
	}

	if temporary {
		requestBody["temporary"] = true
		// For temporary sessions, don't send session ID (server generates it)
		delete(requestBody, "session")
	}

	// Derive authKey from password if provided
	if password != "" {
		authKey := deriveAuthKey(password, createdAt)
		requestBody["authKey"] = authKey
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return Config{}, err
	}

	request, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return Config{}, err
	}

	request.Header.Set("Content-Type", "application/json")

	response, err := client.Do(request)
	if err != nil {
		return Config{}, err
	}
	defer response.Body.Close()

	// Check cookies from the jar (they may have been set on redirect responses)
	parsedURL, _ := url.Parse(apiURL)
	cookies := jar.Cookies(parsedURL)

	var sessionCookie, sessionTokenCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "session" {
			sessionCookie = c
		} else if c.Name == "session_token" {
			sessionTokenCookie = c
		}
	}

	// If not in jar, try response directly (for non-redirect case)
	if sessionCookie == nil {
		for _, c := range response.Cookies() {
			if c.Name == "session" {
				sessionCookie = c
			} else if c.Name == "session_token" {
				sessionTokenCookie = c
			}
		}
	}

	if sessionCookie == nil {
		return Config{}, fmt.Errorf("session cookie not found in response")
	}

	actualSessionID := sessionCookie.Value
	var sessionToken string
	if sessionTokenCookie != nil {
		sessionToken = sessionTokenCookie.Value
	}

	return Config{
		SessionID:    actualSessionID,
		SessionToken: sessionToken,
		CreatedAt:    createdAt,
	}, nil
}

// getTimestamp returns current timestamp in milliseconds (consistent with web app)
func getTimestamp() int64 {
	return time.Now().UnixMilli()
}

func joinSession(data Config, password string) (GetSessionResponseBody, error) {
	// Step 1: Check if session exists and get createdAt
	checkURL := fmt.Sprintf("%s%s?sessionId=%s", utils.API_LINK, utils.API_PATH_SESSION, data.SessionID)
	checkReq, err := http.NewRequest("GET", checkURL, nil)
	if err != nil {
		return GetSessionResponseBody{}, err
	}

	checkResp, err := client.Do(checkReq)
	if err != nil {
		return GetSessionResponseBody{}, err
	}
	defer checkResp.Body.Close()

	var checkData SessionCheckResponse
	if err := json.NewDecoder(checkResp.Body).Decode(&checkData); err != nil {
		return GetSessionResponseBody{}, fmt.Errorf("error parsing check response: %w", err)
	}

	if checkData.CreateNewSession {
		return GetSessionResponseBody{}, errors.New("session does not exist")
	}

	// Step 2: Derive authKey and join session
	requestBody := map[string]interface{}{
		"session": data.SessionID,
		"join":    true,
	}

	if checkData.HasPassword {
		if password == "" {
			return GetSessionResponseBody{}, errors.New("password required for this session")
		}
		authKey := deriveAuthKey(password, checkData.CreatedAt)
		requestBody["authKey"] = authKey
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return GetSessionResponseBody{}, err
	}

	joinURL := utils.API_LINK + utils.API_PATH_SESSION
	joinReq, err := http.NewRequest("POST", joinURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return GetSessionResponseBody{}, err
	}

	joinReq.Header.Set("Content-Type", "application/json")
	joinResp, err := client.Do(joinReq)
	if err != nil {
		return GetSessionResponseBody{}, err
	}
	defer joinResp.Body.Close()

	// Check if we got the session cookie (success) or an error
	parsedURL, _ := url.Parse(joinURL)
	cookies := jar.Cookies(parsedURL)

	var hasSessionCookie bool
	for _, c := range cookies {
		if c.Name == "session" && c.Value == data.SessionID {
			hasSessionCookie = true
			break
		}
	}

	// If not in jar, check response directly
	if !hasSessionCookie {
		for _, c := range joinResp.Cookies() {
			if c.Name == "session" && c.Value == data.SessionID {
				hasSessionCookie = true
				break
			}
		}
	}

	if !hasSessionCookie {
		// Try to parse error response
		var joinResult struct {
			Success bool   `json:"success"`
			Error   string `json:"error"`
		}
		if err := json.NewDecoder(joinResp.Body).Decode(&joinResult); err == nil && !joinResult.Success {
			if joinResult.Error == "invalid_auth_key" || joinResult.Error == "auth_key_required" {
				return GetSessionResponseBody{}, errors.New("invalid password")
			}
			return GetSessionResponseBody{}, fmt.Errorf("failed to join session: %s", joinResult.Error)
		}
		return GetSessionResponseBody{}, errors.New("failed to join session: no session cookie received")
	}

	// Extract session_token from jar
	var sessionToken string
	for _, c := range cookies {
		if c.Name == "session_token" {
			sessionToken = c.Value
			break
		}
	}

	return GetSessionResponseBody{
		SessionID:       data.SessionID,
		HasPassword:     checkData.HasPassword,
		IsValidPassword: true,
		CreatedAt:       checkData.CreatedAt,
		SessionToken:    sessionToken,
	}, nil
}

func createNote(note string, encKey []byte) (*http.Response, error) {
	apiURL := utils.API_LINK + utils.API_PATH_NOTE

	var requestBody map[string]interface{}

	if encKey != nil {
		// Encrypt the note
		encData, err := encryptString(note, encKey)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt note: %w", err)
		}
		requestBody = map[string]interface{}{
			"content":       encData.Ciphertext,
			"isEncrypted":   true,
			"encryptedIv":   encData.IV,
			"encryptedSalt": encData.Salt,
		}
	} else {
		requestBody = map[string]interface{}{
			"content": note,
		}
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}

	r, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}

	r.Header.Set("Content-Type", "application/json")

	response, err := client.Do(r)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func uploadFile(filePath string, encKey []byte) (*http.Response, error) {
	// Step 1: Get file info
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	// Step 2: Get presigned URLs for upload
	presignURL := utils.API_LINK + utils.API_PATH_UPLOAD + "/presign"
	presignBody := map[string]interface{}{
		"files": []map[string]interface{}{
			{
				"name": filepath.Base(filePath),
				"type": "application/octet-stream",
				"size": fileInfo.Size(),
			},
		},
	}

	jsonBody, err := json.Marshal(presignBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", presignURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get presigned URL: %s", string(body))
	}

	var presignData struct {
		Presigned []struct {
			UploadURL string            `json:"uploadUrl"`
			FileKey   string            `json:"fileKey"`
			Headers   map[string]string `json:"headers"`
		} `json:"presigned"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&presignData); err != nil {
		return nil, fmt.Errorf("failed to parse presign response: %w", err)
	}

	if len(presignData.Presigned) == 0 {
		return nil, errors.New("no presigned URL returned")
	}

	// Step 3: Read and optionally encrypt file
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var encryptedData *EncryptedData
	uploadData := fileData

	if encKey != nil {
		// Encrypt file
		encryptedData, err = encryptFile(fileData, encKey)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt file: %w", err)
		}
		// Decode base64 ciphertext to get raw bytes for upload
		uploadData, err = base64.StdEncoding.DecodeString(encryptedData.Ciphertext)
		if err != nil {
			return nil, fmt.Errorf("failed to decode encrypted data: %w", err)
		}
	}

	// Step 4: Upload to R2
	uploadReq, err := http.NewRequest("PUT", presignData.Presigned[0].UploadURL, bytes.NewReader(uploadData))
	if err != nil {
		return nil, err
	}

	// Add headers from presign response (required for signature validation)
	for key, value := range presignData.Presigned[0].Headers {
		uploadReq.Header.Set(key, value)
	}

	uploadResp, err := client.Do(uploadReq)
	if err != nil {
		return nil, fmt.Errorf("failed to upload file: %w", err)
	}
	defer uploadResp.Body.Close()

	if uploadResp.StatusCode != 200 {
		body, _ := io.ReadAll(uploadResp.Body)
		return nil, fmt.Errorf("upload failed: %s", string(body))
	}

	// Step 5: Finalize upload
	finalizeURL := utils.API_LINK + utils.API_PATH_UPLOAD_FINALIZE
	finalizeBody := map[string]interface{}{
		"files": []map[string]interface{}{
			{
				"fileKey":  presignData.Presigned[0].FileKey,
				"fileName": filepath.Base(filePath),
			},
		},
	}

	if encKey != nil && encryptedData != nil {
		finalizeBody["files"].([]map[string]interface{})[0]["isEncrypted"] = true
		finalizeBody["files"].([]map[string]interface{})[0]["encryptedIv"] = encryptedData.IV
		finalizeBody["files"].([]map[string]interface{})[0]["encryptedSalt"] = encryptedData.Salt
	}

	jsonBody, err = json.Marshal(finalizeBody)
	if err != nil {
		return nil, err
	}

	finalizeReq, err := http.NewRequest("POST", finalizeURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}
	finalizeReq.Header.Set("Content-Type", "application/json")

	return client.Do(finalizeReq)
}

func getSessionContent() ([]ContentType, error) {
	apiURL := utils.API_LINK + utils.API_PATH_CONTENT

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return nil, errors.New("unauthorized: not logged in or session expired")
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API error: status %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)

	var rawData []json.RawMessage
	if err := json.Unmarshal(body, &rawData); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}

	var results []ContentType
	for _, item := range rawData {
		var generic struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(item, &generic); err != nil {
			return nil, err
		}

		switch generic.Type {
		case "note":
			var note NoteType
			if err := json.Unmarshal(item, &note); err != nil {
				return nil, err
			}
			results = append(results, note)

		case "attachment":
			var attachment AttachmentType
			if err := json.Unmarshal(item, &attachment); err != nil {
				return nil, err
			}
			results = append(results, attachment)
		}
	}

	return results, nil
}

func getSessionStatus() (*SessionCheckResponse, error) {
	config, err := getConfig()
	if err != nil {
		return nil, err
	}

	apiURL := fmt.Sprintf("%s%s?sessionId=%s", utils.API_LINK, utils.API_PATH_SESSION, config.SessionID)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get session status: %d", resp.StatusCode)
	}

	var status SessionCheckResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("failed to parse session status: %w", err)
	}

	return &status, nil
}

func enableEncryption(password string) error {
	config, err := getConfig()
	if err != nil {
		return err
	}

	// Load cookies from config into jar
	loadCookiesToJar(config)

	// First, verify password
	status, err := getSessionStatus()
	if err != nil {
		return err
	}

	if !status.HasPassword {
		return errors.New("session must have a password to enable E2EE")
	}

	// Derive auth key and verify password
	authKey := deriveAuthKey(password, config.CreatedAt)
	verifyURL := fmt.Sprintf("%s%s/verify-password?sessionId=%s", utils.API_LINK, utils.API_PATH_SESSION, config.SessionID)
	verifyBody := map[string]string{"authKey": authKey}
	jsonBody, _ := json.Marshal(verifyBody)

	req, err := http.NewRequest("POST", verifyURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var verifyResult struct {
		Valid bool `json:"valid"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&verifyResult); err != nil {
		return fmt.Errorf("failed to parse verification: %w", err)
	}

	if !verifyResult.Valid {
		return errors.New("invalid password")
	}

	// Enable encryption on server
	encURL := utils.API_LINK + "/sessions/encryption"
	encBody := map[string]bool{"isEncrypted": true}
	jsonBody, _ = json.Marshal(encBody)

	req, err = http.NewRequest("POST", encURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err = client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to enable encryption: %s", string(body))
	}

	// Store encryption key locally
	encKey := deriveEncKey(password, config.CreatedAt)
	config.EncKey = bytesToHex(encKey)
	return writeConfig(config)
}

func disableEncryption() error {
	config, err := getConfig()
	if err != nil {
		return err
	}

	// Load cookies from config into jar
	loadCookiesToJar(config)

	encURL := utils.API_LINK + "/sessions/encryption"
	encBody := map[string]bool{"isEncrypted": false}
	jsonBody, _ := json.Marshal(encBody)

	req, err := http.NewRequest("POST", encURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to disable encryption: %s", string(body))
	}

	// Remove encryption key from config
	config, err = getConfig()
	if err != nil {
		return err
	}
	config.EncKey = ""
	return writeConfig(config)
}

func getContentByID(contentID string) (ContentType, error) {
	contents, err := getSessionContent()
	if err != nil {
		return nil, err
	}

	for _, content := range contents {
		if content.GetID() == contentID {
			return content, nil
		}
	}

	return nil, errors.New("content not found")
}

func deleteContent(contentID string) error {
	apiURL := utils.API_LINK + utils.API_PATH_CONTENT + "?contentId=" + contentID

	req, err := http.NewRequest("DELETE", apiURL, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete failed: %s", string(body))
	}

	return nil
}

func downloadFile(contentID string, outputPath string, encKey []byte) error {
	content, err := getContentByID(contentID)
	if err != nil {
		return err
	}

	attachment, ok := content.(AttachmentType)
	if !ok {
		return errors.New("content is not a file")
	}

	downloadURL := fmt.Sprintf("https://copyman.fr/api/content/download/%s", contentID)

	req, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}

	// Read downloaded data
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read download data: %w", err)
	}

	// Decrypt if file is encrypted and we have the key
	if attachment.IsEncrypted {
		if encKey == nil {
			return errors.New("file is encrypted but no encryption key available - login with password to decrypt")
		}
		encData := &EncryptedData{
			Ciphertext: base64.StdEncoding.EncodeToString(data),
			IV:         attachment.EncryptedIv,
			Salt:       attachment.EncryptedSalt,
		}
		data, err = decryptFile(encData, encKey)
		if err != nil {
			return fmt.Errorf("failed to decrypt file: %w", err)
		}
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = out.Write(data)
	return err
}

func remove(slice []string, s int) []string {
	return append(slice[:s], slice[s+1:]...)
}

func getKeyValFromStr(value string) ([]KeyValue, error) {
	var keyVals []KeyValue = []KeyValue{}

	var lines []string = strings.Split(value, "\n")
	for _, line := range lines {
		var splitedLine []string = strings.Split(line, "=")
		if len(splitedLine) < 2 {
			continue
		}
		var key = splitedLine[0]
		var value string = strings.Join(remove(splitedLine, 0), "=")
		keyVals = append(keyVals, KeyValue{Key: key, Value: value})
	}
	return keyVals, nil
}

func getFolderConfigPath() (string, error) {
	configDir, err := os.UserConfigDir()

	if err != nil {
		return "", err
	}

	return configDir + configFolderPath, nil
}

func getConfigPath() (string, error) {
	configDir, err := os.UserConfigDir()

	if err != nil {
		return "", err
	}

	return configDir + configFolderPath + configFolderFileName, nil
}

func createDefaultConfig() error {
	configFolderPath, err := getFolderConfigPath()
	if err != nil {
		return err
	}
	configFilePath, err := getConfigPath()

	if err != nil {
		return err
	}

	err = os.MkdirAll(configFolderPath, os.ModePerm)

	if err != nil {
		return err
	}

	if _, err := os.Stat(configFilePath); errors.Is(err, os.ErrNotExist) {
		_, err = os.Create(configFilePath)

		if err != nil {
			return err
		}
	}

	return nil
}

func writeConfig(config Config) error {
	path, err := getConfigPath()
	if err != nil {
		return err
	}

	v := reflect.ValueOf(config)

	typeOfS := v.Type()

	configStr := ""
	for i := 0; i < v.NumField(); i++ {
		str := fmt.Sprintf("%s=%v\n", typeOfS.Field(i).Name, v.Field(i).Interface())
		configStr += str
	}
	d1 := []byte(configStr)
	err = os.WriteFile(path, d1, 0644)

	if err != nil {
		return err
	}

	return nil
}

func loadCookiesToJar(config Config) {
	if config.SessionID == "" {
		return
	}

	// Create cookies for the API domain
	apiURL, _ := url.Parse(utils.API_LINK)
	cookies := []*http.Cookie{}

	// Add session cookie
	cookies = append(cookies, &http.Cookie{
		Name:     "session",
		Value:    config.SessionID,
		Path:     "/",
		Domain:   apiURL.Hostname(),
		HttpOnly: false,
	})

	// Add session_token if available
	if config.SessionToken != "" {
		cookies = append(cookies, &http.Cookie{
			Name:     "session_token",
			Value:    config.SessionToken,
			Path:     "/",
			Domain:   apiURL.Hostname(),
			HttpOnly: true,
		})
	}

	jar.SetCookies(apiURL, cookies)
}

func getConfig() (Config, error) {
	path, err := getConfigPath()

	if err != nil {
		return Config{}, err
	}
	rawConfig, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	s := string(rawConfig)

	keyVals, err := getKeyValFromStr(s)

	if err != nil {
		return Config{}, err
	}
	config := Config{}

	for _, keyVal := range keyVals {
		switch keyVal.Key {
		case "SessionID":
			config.SessionID = keyVal.Value
		case "SessionToken":
			config.SessionToken = keyVal.Value
		case "CreatedAt":
			config.CreatedAt = keyVal.Value
		case "EncKey":
			config.EncKey = keyVal.Value
		}
	}

	return config, nil
}

func sanitize(s string) string {
	return strings.ReplaceAll(s, "\n", " ")
}

func truncate(s string, maxLength int) string {
	s = sanitize(s)
	if len(s) > maxLength {
		return s[:maxLength] + "..."
	}
	return s
}

func printHelp() {
	fmt.Println(`copyman - A CLI tool for copyman.fr

Usage:
  copyman <command> [options]

Commands:
  create    Create a new session
            copyman create [--session-id <id>] [--password <pass>] [--temp]
  
  login     Login to an existing session
            copyman login --session-id <id> --password <pass>
  
  logout    Disconnect from session
            copyman logout
  
  push      Upload content to session
            copyman push text "your text content"
            copyman push file /path/to/file
  
  list      List content in session
            copyman list [--json]
  
  get       Get content by ID
            copyman get <content-id> [--output /path]
            copyman get -i              # Interactive mode with arrow key selection
  
  delete    Delete content by ID
            copyman delete <content-id>

  status    Show session status and E2EE info
            copyman status

  encryption Manage E2EE encryption
            copyman encryption enable --password <pass>
            copyman encryption disable

  update    Check for updates and self-update
            copyman update

  version   Show version information
            copyman version
            copyman --version

Options:
  --session-id    Session ID (custom or to join)
  --password      Password for session
  --temp          Create a temporary session (auto-expires)
  --json          Output in JSON format
  --output        Output file path for downloads
  -i              Interactive mode (for get command)
  --help, -h      Show help`)
}

func outputJSON(v interface{}) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(v)
}

func toContentOutput(content ContentType) ContentOutput {
	output := ContentOutput{
		ID:   content.GetID(),
		Type: content.GetType(),
	}

	switch c := content.(type) {
	case NoteType:
		output.Content = c.Content
		output.CreatedAt = int64(c.CreatedAt)
		output.UpdatedAt = int64(c.UpdatedAt)
	case AttachmentType:
		output.AttachmentURL = c.AttachmentURL
		output.AttachmentPath = c.AttachmentPath
		output.CreatedAt = int64(c.CreatedAt)
		output.UpdatedAt = int64(c.UpdatedAt)
	}

	return output
}

func runCreate(args []string) error {
	fs := flag.NewFlagSet("create", flag.ExitOnError)
	sessionID := fs.String("session-id", "", "Custom session ID (optional)")
	password := fs.String("password", "", "Password (optional)")
	temp := fs.Bool("temp", false, "Create a temporary session (auto-generated ID, expires after some time)")

	fs.Parse(args)

	var config Config
	var err error

	if *temp {
		fmt.Println("Creating temporary session...")
		config, err = createSession("", *password, true)
	} else {
		id := *sessionID
		if id == "" {
			id = "ai" + generateRandomID(10)
		}
		fmt.Printf("Creating session with ID: %s\n", id)
		config, err = createSession(id, *password, false)
	}

	if err != nil {
		return fmt.Errorf("cannot create session: %w", err)
	}

	err = writeConfig(config)
	if err != nil {
		return fmt.Errorf("cannot write to config file: %w", err)
	}

	if *temp {
		fmt.Printf("Temporary session created!\nSession ID: %s\n", config.SessionID)
	} else {
		fmt.Printf("Session created!\nSession ID: %s\n", config.SessionID)
	}
	if *password != "" {
		fmt.Printf("Password: %s\n", *password)
		// Derive encryption key from password
		encKey := deriveEncKey(*password, config.CreatedAt)
		config.EncKey = bytesToHex(encKey)
		err = writeConfig(config)
		if err != nil {
			return fmt.Errorf("cannot write encryption key to config: %w", err)
		}
	}
	fmt.Println("\nShare this session ID with others to collaborate!")
	return nil
}

func generateRandomID(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, length)
	for i := 0; i < length; i++ {
		result[i] = charset[rand.Intn(len(charset))]
	}
	return string(result)
}

func runLogin(args []string) error {
	fs := flag.NewFlagSet("login", flag.ExitOnError)
	sessionID := fs.String("session-id", "", "Session ID")
	password := fs.String("password", "", "Password")

	fs.Parse(args)

	if *sessionID == "" {
		return errors.New("login requires --session-id")
	}

	joinedSession, err := joinSession(Config{SessionID: *sessionID}, *password)
	if err != nil {
		return fmt.Errorf("cannot join session: %w", err)
	}

	config := Config{
		SessionID:    joinedSession.SessionID,
		CreatedAt:    joinedSession.CreatedAt,
		SessionToken: joinedSession.SessionToken,
	}

	// Derive encryption key from password if provided
	if *password != "" {
		encKey := deriveEncKey(*password, joinedSession.CreatedAt)
		config.EncKey = bytesToHex(encKey)
	}

	err = writeConfig(config)
	if err != nil {
		return fmt.Errorf("cannot write to config file: %w", err)
	}

	fmt.Println("Successfully joined session!")
	return nil
}

func runLogout() error {
	config, err := getConfig()
	if err != nil {
		return fmt.Errorf("cannot get config: %w", err)
	}

	if config.SessionID == "" {
		fmt.Println("You're not logged in")
		return nil
	}

	// Call server logout endpoint to clear session token
	logoutURL := utils.API_LINK + utils.API_PATH_SESSION
	req, err := http.NewRequest("DELETE", logoutURL, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()

	err = writeConfig(Config{
		SessionID:    "",
		SessionToken: "",
		CreatedAt:    "",
	})
	if err != nil {
		return err
	}

	fmt.Println("Successfully logged out")
	return nil
}

func runPush(args []string) error {
	config, err := getConfig()
	if err != nil {
		return fmt.Errorf("cannot get config: %w", err)
	}

	if config.SessionID == "" {
		return errors.New("not logged in. Run 'copyman login' first")
	}

	// Load cookies from config into jar
	loadCookiesToJar(config)

	// Check if session has E2EE enabled
	var encKey []byte
	status, err := getSessionStatus()
	if err != nil {
		return fmt.Errorf("failed to get session status: %w", err)
	}

	// Only use encryption key if session has E2EE enabled
	if status.IsEncrypted && config.EncKey != "" {
		encKey, err = hexToBytes(config.EncKey)
		if err != nil {
			return fmt.Errorf("invalid encryption key in config: %w", err)
		}
	}

	if len(args) < 2 {
		return errors.New("push requires subcommand: 'text' or 'file'")
	}

	subCommand := args[0]
	remainingArgs := args[1:]

	switch subCommand {
	case "text":
		if len(remainingArgs) == 0 {
			return errors.New("push text requires content argument")
		}
		text := strings.Join(remainingArgs, " ")
		resp, err := createNote(text, encKey)
		if err != nil {
			return fmt.Errorf("error creating note: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("failed to create note: %s", string(body))
		}
		fmt.Println("Successfully pushed text to your session")

	case "file":
		if len(remainingArgs) == 0 {
			return errors.New("push file requires file path argument")
		}
		for _, filePath := range remainingArgs {
			resp, err := uploadFile(filePath, encKey)
			if err != nil {
				return fmt.Errorf("error uploading file %s: %w", filePath, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("failed to upload file %s: %s", filePath, string(body))
			}
		}
		fmt.Printf("Successfully pushed %d file(s) to your session\n", len(remainingArgs))

	default:
		return fmt.Errorf("unknown push subcommand: %s (use 'text' or 'file')", subCommand)
	}

	return nil
}

func runList(args []string) error {
	config, err := getConfig()
	if err != nil {
		return fmt.Errorf("cannot get config: %w", err)
	}

	if config.SessionID == "" {
		return errors.New("not logged in. Run 'copyman login' first")
	}

	// Load cookies from config into jar
	loadCookiesToJar(config)

	fs := flag.NewFlagSet("list", flag.ExitOnError)
	jsonOutput := fs.Bool("json", false, "Output in JSON format")
	fs.Parse(args)

	contents, err := getSessionContent()
	if err != nil {
		return fmt.Errorf("error getting session content: %w", err)
	}

	if *jsonOutput {
		var outputs []ContentOutput
		for _, content := range contents {
			outputs = append(outputs, toContentOutput(content))
		}
		return outputJSON(outputs)
	}

	for _, content := range contents {
		switch c := content.(type) {
		case NoteType:
			encIndicator := ""
			if c.IsEncrypted {
				encIndicator = " [ENCRYPTED]"
			}
			fmt.Printf("[TEXT]%s %s | %s\n", encIndicator, c.ID, truncate(c.Content, 50))
		case AttachmentType:
			encIndicator := ""
			if c.IsEncrypted {
				encIndicator = " [ENCRYPTED]"
			}
			fmt.Printf("[FILE]%s %s | %s\n", encIndicator, c.ID, c.AttachmentPath)
		}
	}

	return nil
}

func runGet(args []string) error {
	config, err := getConfig()
	if err != nil {
		return fmt.Errorf("cannot get config: %w", err)
	}

	if config.SessionID == "" {
		return errors.New("not logged in. Run 'copyman login' first")
	}

	// Load cookies from config into jar
	loadCookiesToJar(config)

	// Check if session has E2EE enabled
	var encKey []byte
	status, err := getSessionStatus()
	if err != nil {
		return fmt.Errorf("failed to get session status: %w", err)
	}

	// Only use encryption key if session has E2EE enabled
	if status.IsEncrypted && config.EncKey != "" {
		encKey, err = hexToBytes(config.EncKey)
		if err != nil {
			return fmt.Errorf("invalid encryption key in config: %w", err)
		}
	}

	fs := flag.NewFlagSet("get", flag.ExitOnError)
	output := fs.String("output", "", "Output file path for downloads")
	interactive := fs.Bool("i", false, "Interactive mode - select from list with arrow keys")
	fs.Parse(args)

	remaining := fs.Args()

	var contentID string
	var selectedContent ContentType

	if *interactive || len(remaining) == 0 {
		// Interactive mode - show selectable list
		contents, err := getSessionContent()
		if err != nil {
			return fmt.Errorf("error getting session content: %w", err)
		}

		if len(contents) == 0 {
			return errors.New("no content available in session")
		}

		// Prepare items for selection
		items := make([]string, len(contents))
		for i, content := range contents {
			switch c := content.(type) {
			case NoteType:
				items[i] = fmt.Sprintf("[TEXT] %s | %s", c.ID, truncate(c.Content, 40))
			case AttachmentType:
				items[i] = fmt.Sprintf("[FILE] %s | %s", c.ID, c.AttachmentPath)
			}
		}

		// Create prompt for selection
		prompt := promptui.Select{
			Label: "Select content to download",
			Items: items,
			Size:  10,
		}

		index, _, err := prompt.Run()
		if err != nil {
			return fmt.Errorf("selection cancelled: %w", err)
		}

		selectedContent = contents[index]
		contentID = selectedContent.GetID()

		// For attachments, prompt for custom filename
		if attachment, ok := selectedContent.(AttachmentType); ok {
			filenamePrompt := promptui.Prompt{
				Label:   "Enter filename",
				Default: attachment.AttachmentPath,
			}

			customName, err := filenamePrompt.Run()
			if err != nil {
				return fmt.Errorf("filename prompt cancelled: %w", err)
			}

			if customName != "" {
				*output = customName
			}
		}
	} else {
		contentID = remaining[0]
		content, err := getContentByID(contentID)
		if err != nil {
			return fmt.Errorf("error getting content: %w", err)
		}
		selectedContent = content
	}

	switch c := selectedContent.(type) {
	case NoteType:
		if c.IsEncrypted {
			if encKey == nil {
				return errors.New("note is encrypted but no encryption key available - login with password to decrypt")
			}
			encData := &EncryptedData{
				Ciphertext: c.Content,
				IV:         c.EncryptedIv,
				Salt:       c.EncryptedSalt,
			}
			decrypted, err := decryptString(encData, encKey)
			if err != nil {
				return fmt.Errorf("failed to decrypt note: %w", err)
			}
			fmt.Println(decrypted)
		} else {
			fmt.Println(c.Content)
		}
	case AttachmentType:
		if *output == "" {
			*output = c.AttachmentPath
		}
		err := downloadFile(contentID, *output, encKey)
		if err != nil {
			return fmt.Errorf("error downloading file: %w", err)
		}
		fmt.Printf("Downloaded to: %s\n", *output)
	}

	return nil
}

func runDelete(args []string) error {
	config, err := getConfig()
	if err != nil {
		return fmt.Errorf("cannot get config: %w", err)
	}

	if config.SessionID == "" {
		return errors.New("not logged in. Run 'copyman login' first")
	}

	// Load cookies from config into jar
	loadCookiesToJar(config)

	if len(args) == 0 {
		return errors.New("delete requires content-id argument")
	}

	contentID := args[0]

	err = deleteContent(contentID)
	if err != nil {
		return fmt.Errorf("error deleting content: %w", err)
	}

	fmt.Printf("Successfully deleted content: %s\n", contentID)
	return nil
}

func main() {
	err := createDefaultConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Cannot create config file: %v\n", err)
		os.Exit(1)
	}

	if len(os.Args) < 2 {
		printHelp()
		os.Exit(1)
	}

	command := os.Args[1]
	args := os.Args[2:]

	switch command {
	case "create":
		err = runCreate(args)
	case "login":
		err = runLogin(args)
	case "logout":
		err = runLogout()
	case "push":
		err = runPush(args)
	case "list":
		err = runList(args)
	case "get":
		err = runGet(args)
	case "delete":
		err = runDelete(args)
	case "status":
		err = runStatus()
	case "encryption":
		err = runEncryption(args)
	case "update":
		err = runUpdate(args)
	case "version", "--version", "-v":
		runVersion()
	case "help", "-h", "--help":
		printHelp()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		printHelp()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runStatus() error {
	config, err := getConfig()
	if err != nil {
		return fmt.Errorf("cannot get config: %w", err)
	}

	if config.SessionID == "" {
		return errors.New("not logged in. Run 'copyman login' first")
	}

	status, err := getSessionStatus()
	if err != nil {
		return fmt.Errorf("failed to get session status: %w", err)
	}

	hasEncKey := config.EncKey != ""

	fmt.Printf("Session ID: %s\n", config.SessionID)
	fmt.Printf("Has Password: %t\n", status.HasPassword)
	fmt.Printf("E2EE Enabled: %t\n", status.IsEncrypted)
	fmt.Printf("Encryption Key Available: %t\n", hasEncKey)
	fmt.Printf("Created At: %s\n", status.CreatedAt)

	if status.HasPassword && !status.IsEncrypted {
		fmt.Println("\nNote: Session has password but E2EE is not enabled.")
		fmt.Println("Run 'copyman encryption enable' to enable E2EE.")
	}

	if status.IsEncrypted && !hasEncKey {
		fmt.Println("\nWarning: Session has E2EE enabled but no encryption key.")
		fmt.Println("Login with password to derive the encryption key.")
	}

	return nil
}

func runEncryption(args []string) error {
	if len(args) == 0 {
		return errors.New("encryption requires subcommand: 'enable' or 'disable'")
	}

	subCommand := args[0]

	switch subCommand {
	case "enable":
		fs := flag.NewFlagSet("encryption enable", flag.ExitOnError)
		password := fs.String("password", "", "Session password (required)")
		fs.Parse(args[1:])

		if *password == "" {
			return errors.New("--password is required to enable E2EE")
		}

		fmt.Println("Enabling E2EE encryption...")
		if err := enableEncryption(*password); err != nil {
			return fmt.Errorf("failed to enable encryption: %w", err)
		}
		fmt.Println("E2EE encryption enabled successfully!")
		fmt.Println("Future content will be encrypted before upload.")

	case "disable":
		fmt.Println("Disabling E2EE encryption...")
		if err := disableEncryption(); err != nil {
			return fmt.Errorf("failed to disable encryption: %w", err)
		}
		fmt.Println("E2EE encryption disabled.")
		fmt.Println("Note: Previously encrypted content remains encrypted.")

	default:
		return fmt.Errorf("unknown encryption subcommand: %s (use 'enable' or 'disable')", subCommand)
	}

	return nil
}
