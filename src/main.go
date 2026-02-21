package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/mathysin/copyman-cli/utils"
)

const configFolderPath = "/copyman/"
const configFolderFileName = "config"

var jar, _ = cookiejar.New(nil)

var client = http.Client{
	Jar: jar,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

type GetSessionResponseBody struct {
	SessionID        string `json:"sessionId"`
	Password         string `json:"password"`
	CreateNewSession bool   `json:"createNewSession"`
	HasPassword      bool   `json:"hasPassword"`
	IsValidPassword  bool   `json:"isValidPassword"`
}

type PostNoteRequestBody struct {
	Content string `json:"content"`
}

type KeyValue struct {
	Key   string
	Value string
}

type Config struct {
	SessionID string
	Password  string
}

type BaseContentType struct {
	ID        string    `json:"id"`
	CreatedAt Timestamp `json:"createdAt"`
	UpdatedAt Timestamp `json:"updatedAt"`
	Type      string    `json:"type"`
}

type NoteType struct {
	BaseContentType
	Content string `json:"content"`
}

type AttachmentType struct {
	BaseContentType
	AttachmentURL  string `json:"attachmentURL"`
	AttachmentPath string `json:"attachmentPath"`
	FileKey        string `json:"fileKey"`
}

type ContentOutput struct {
	ID             string `json:"id"`
	Type           string `json:"type"`
	Content        string `json:"content,omitempty"`
	AttachmentURL  string `json:"attachmentUrl,omitempty"`
	AttachmentPath string `json:"attachmentPath,omitempty"`
	CreatedAt      int64  `json:"createdAt"`
	UpdatedAt      int64  `json:"updatedAt"`
}

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

type ContentType interface {
	GetType() string
	GetID() string
}

func (n NoteType) GetType() string {
	return n.Type
}

func (n NoteType) GetID() string {
	return n.ID
}

func (a AttachmentType) GetType() string {
	return a.Type
}

func (a AttachmentType) GetID() string {
	return a.ID
}

func createSession(sessionID string, password string, temporary bool) (Config, error) {
	apiURL := utils.API_LINK + utils.API_PATH_SESSION

	formData := url.Values{}
	if !temporary {
		formData.Set("session", strings.ToLower(sessionID))
	}
	if password != "" {
		formData.Set("password", password)
	}
	if temporary {
		formData.Set("temporary", "true")
	}

	request, err := http.NewRequest("POST", apiURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return Config{}, err
	}

	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	response, err := client.Do(request)
	if err != nil {
		return Config{}, err
	}
	defer response.Body.Close()

	body, _ := io.ReadAll(response.Body)

	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return Config{}, fmt.Errorf("error parsing response: %w (body: %s)", err, string(body))
	}

	if !result.Success {
		return Config{}, fmt.Errorf("failed to create session: %s", result.Error)
	}

	cookies := response.Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "session" {
			sessionCookie = c
			break
		}
	}

	if sessionCookie == nil {
		return Config{}, fmt.Errorf("session cookie not found in response")
	}

	actualSessionID := sessionCookie.Value
	hashedPassword := password

	var passwordCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "password" {
			passwordCookie = c
			break
		}
	}
	if passwordCookie != nil {
		hashedPassword = passwordCookie.Value
	}

	return Config{SessionID: actualSessionID, Password: hashedPassword}, nil
}

func joinSession(data Config) (GetSessionResponseBody, error) {
	apiURL := fmt.Sprintf(utils.API_LINK+utils.API_PATH_SESSION+"?sessionId=%s&password=%s", data.SessionID, data.Password)

	request, err := http.NewRequest("GET", apiURL, nil)

	if err != nil {
		return GetSessionResponseBody{}, err
	}

	request.Header.Add(utils.HEADER_CONTENT_TYPE, utils.CONTENT_TYPE_FORM)
	response, err := client.Do(request)

	if err != nil {
		return GetSessionResponseBody{}, err
	}

	defer response.Body.Close()
	var sessionCookieData GetSessionResponseBody
	if err := json.NewDecoder(response.Body).Decode(&sessionCookieData); err != nil {
		return GetSessionResponseBody{}, errors.New("error parsing server response")
	}

	if sessionCookieData.CreateNewSession || !sessionCookieData.IsValidPassword {
		return GetSessionResponseBody{}, errors.New("invalid sessionId or password")
	}

	return sessionCookieData, nil
}

func createNote(data Config, note string) (*http.Response, error) {
	apiURL := utils.API_LINK + utils.API_PATH_NOTE
	body := []byte(`{
		"content":"` + note + `"
	}`)

	r, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	r.Header.Set("Content-Type", "application/json")

	cookies := []*http.Cookie{
		{Name: "session", Value: data.SessionID},
		{Name: "password", Value: data.Password},
	}

	for _, cookie := range cookies {
		r.AddCookie(cookie)
	}

	response, err := client.Do(r)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func uploadFile(data Config, filePath string) (*http.Response, error) {
	apiURL := "https://copyman.fr/api/content/upload"

	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("files", filepath.Base(filePath))
	if err != nil {
		return nil, err
	}

	if _, err = io.Copy(part, file); err != nil {
		return nil, err
	}

	if err = writer.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", apiURL, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	cookies := []*http.Cookie{
		{Name: "session", Value: data.SessionID},
		{Name: "password", Value: data.Password},
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}

	return client.Do(req)
}

func getSessionContent(data Config) ([]ContentType, error) {
	apiURL := utils.API_LINK + utils.API_PATH_CONTENT

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	cookies := []*http.Cookie{
		{Name: "session", Value: data.SessionID},
		{Name: "password", Value: data.Password},
	}
	for _, cookie := range cookies {
		req.AddCookie(cookie)
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

func getContentByID(data Config, contentID string) (ContentType, error) {
	contents, err := getSessionContent(data)
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

func deleteContent(data Config, contentID string) error {
	apiURL := utils.API_LINK + utils.API_PATH_CONTENT + "?contentId=" + contentID

	req, err := http.NewRequest("DELETE", apiURL, nil)
	if err != nil {
		return err
	}

	cookies := []*http.Cookie{
		{Name: "session", Value: data.SessionID},
		{Name: "password", Value: data.Password},
	}
	for _, cookie := range cookies {
		req.AddCookie(cookie)
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

func downloadFile(data Config, contentID string, outputPath string) error {
	content, err := getContentByID(data, contentID)
	if err != nil {
		return err
	}

	_, ok := content.(AttachmentType)
	if !ok {
		return errors.New("content is not a file")
	}

	downloadURL := fmt.Sprintf("https://copyman.fr/api/content/download/%s", contentID)

	req, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		return err
	}

	cookies := []*http.Cookie{
		{Name: "session", Value: data.SessionID},
		{Name: "password", Value: data.Password},
	}
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
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
		case "Password":
			config.Password = keyVal.Value
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
  
  delete    Delete content by ID
            copyman delete <content-id>

Options:
  --session-id    Session ID (custom or to join)
  --password      Password for session
  --temp          Create a temporary session (auto-expires)
  --json          Output in JSON format
  --output        Output file path for downloads
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

	joinedSession, err := joinSession(Config{SessionID: *sessionID, Password: *password})
	if err != nil {
		return fmt.Errorf("cannot join session: %w", err)
	}

	err = writeConfig(Config{
		SessionID: joinedSession.SessionID,
		Password:  joinedSession.Password,
	})

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

	err = writeConfig(Config{
		SessionID: "",
		Password:  "",
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
		resp, err := createNote(config, text)
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
			resp, err := uploadFile(config, filePath)
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

	fs := flag.NewFlagSet("list", flag.ExitOnError)
	jsonOutput := fs.Bool("json", false, "Output in JSON format")
	fs.Parse(args)

	contents, err := getSessionContent(config)
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
			fmt.Printf("[TEXT] %s | %s\n", c.ID, truncate(c.Content, 50))
		case AttachmentType:
			fmt.Printf("[FILE] %s | %s\n", c.ID, c.AttachmentPath)
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

	fs := flag.NewFlagSet("get", flag.ExitOnError)
	output := fs.String("output", "", "Output file path for downloads")
	fs.Parse(args)

	remaining := fs.Args()
	if len(remaining) == 0 {
		return errors.New("get requires content-id argument")
	}

	contentID := remaining[0]

	content, err := getContentByID(config, contentID)
	if err != nil {
		return fmt.Errorf("error getting content: %w", err)
	}

	switch c := content.(type) {
	case NoteType:
		fmt.Println(c.Content)
	case AttachmentType:
		if *output == "" {
			*output = c.AttachmentPath
		}
		err := downloadFile(config, contentID, *output)
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

	if len(args) == 0 {
		return errors.New("delete requires content-id argument")
	}

	contentID := args[0]

	err = deleteContent(config, contentID)
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
