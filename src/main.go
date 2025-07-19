package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"os"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/manifoldco/promptui"

	socketio_v5 "github.com/maldikhan/go.socket.io"
	socketio_v5_client "github.com/maldikhan/go.socket.io/socket.io/v5/client"
	"github.com/mathysin/copyman-cli/utils"
	"golang.org/x/term"
)

func main2() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	clientSocket, err := socketio_v5.NewClient(
		socketio_client_v5.WithRawURL(utils.SITE_LINK),
	)
	if err != nil {
		log.Fatalf("Error creating client: %v", err)
	}

	clientSocket.On("eventname", func(data []byte) {
		fmt.Printf("Received message: %s\n", string(data))
	})

	if err := clientSocket.Connect(ctx); err != nil {
		log.Fatalf("Error connecting to server: %v", err)
	}

	if err := clientSocket.Emit("hello", "world"); err != nil {
		log.Fatalf("Error sending message: %v", err)
	}

	<-ctx.Done()
}

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
type PostSessionRequestBody struct {
	Session  string `json:"session"`
	Create   bool   `json:"create"`
	Password string `json:"password"`
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

type SessionCookieData struct {
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

type Timestamp time.Time

func (t *Timestamp) UnmarshalJSON(b []byte) error {
	var raw interface{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}

	switch v := raw.(type) {
	case float64:
		*t = Timestamp(time.UnixMilli(int64(v)))
	case string:
		parsedTime, err := time.Parse(time.RFC3339, v)
		if err == nil {
			*t = Timestamp(parsedTime)
			return nil
		}

		ms, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid timestamp format: %v", v)
		}
		*t = Timestamp(time.UnixMilli(ms))
	default:
		return fmt.Errorf("invalid timestamp format: %v", raw)
	}
	return nil

}

func (t Timestamp) Time() time.Time {
	return time.Time(t)
}

type ContentType interface {
	GetType() string
}

func (n NoteType) GetType() string {
	return n.Type
}

func (a AttachmentType) GetType() string {
	return a.Type
}

type Command string

const (
	NONE   Command = "none"
	LOGIN  Command = "login"
	LOGOUT Command = "logout"
	PUSH   Command = "push"
	LIST   Command = "list"
)

func joinSession(data Config) (GetSessionResponseBody, error) {
	url := fmt.Sprintf(utils.API_LINK+utils.API_PATH_SESSION+"?sessionId=%s&password=%s", data.SessionID, data.Password)

	request, err := http.NewRequest("GET", url, nil)

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
		return GetSessionResponseBody{}, errors.New("invalid sessionId")
	}

	return sessionCookieData, nil
}

func createNote(data Config, note string) (*http.Response, error) {
	url := utils.API_LINK + utils.API_PATH_NOTE
	body := []byte(`{
		"content":"` + note + `"
	}`)

	r, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	cookies := []*http.Cookie{
		{Name: "session", Value: data.SessionID},
		{Name: "password", Value: data.Password},
	}

	for _, cookie := range cookies {
		r.AddCookie(cookie)
	}

	if err != nil {
		return nil, err
	}

	response, err := client.Do(r)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func getSessionContent(data Config) ([]ContentType, error) {
	url := utils.API_LINK + utils.API_PATH_CONTENT

	req, err := http.NewRequest("GET", url, nil)
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

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var rawData []json.RawMessage
	if err := json.Unmarshal(body, &rawData); err != nil {
		return nil, err
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
		default:
			fmt.Printf("Unknown config key: %s\n", keyVal.Key)
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

func downloadFile(url string) error {
	prompt := promptui.Prompt{
		Label: "Enter file save location",
	}

	filename, err := prompt.Run()
	if err != nil {
		return err
	}

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	if err == nil {
		fmt.Println("Download completed:", filename)
	}
	return err
}

func main() {

	err := createDefaultConfig()
	if err != nil {
		fmt.Println("Cannot create config file:", err)
		return
	}
	config, err := getConfig()
	if err != nil {
		fmt.Println("Cannot get config file:", err)
		return
	}

	if len(os.Args) < 2 {
		fmt.Println("Missing arguments")
		fmt.Println("")
		fmt.Println("Commands :")
		fmt.Println("- " + LOGIN)
		fmt.Println("- " + LOGOUT)
		fmt.Println("- " + PUSH)
		fmt.Println("- " + LIST)
		return
	}

	contentType := os.Args[1]

	var command Command = NONE

	switch strings.ToLower(contentType) {
	case "login":
		command = LOGIN
	case "list":
		command = LIST
	case "logout":
		command = LOGOUT
	case "push":
		command = PUSH
	}

	if command == NONE {
		fmt.Println(command)
		return
	}

	switch command {
	case LOGOUT:
		if config.SessionID == "" {
			fmt.Println("You're not logged in")
			return
		}
		err = writeConfig(Config{
			SessionID: "",
			Password:  "",
		})
		fmt.Println("Logged out")
		return
	case LOGIN:
		if len(os.Args) < 3 {
			fmt.Println("login <sessionId>")
			return
		}

		inputSessionId := os.Args[2]

		fmt.Print("Enter Password: ")

		inputPassword, err := term.ReadPassword(int(syscall.Stdin))
		if err != nil {
			fmt.Println("Error reading password:", err)
			return
		}

		joinedSession, err := joinSession(Config{SessionID: inputSessionId, Password: string(inputPassword)})
		if err != nil {
			fmt.Println("Cannot join session", err)
			return
		}

		err = writeConfig(Config{
			SessionID: inputSessionId,
			Password:  joinedSession.Password,
		})

		if err != nil {
			fmt.Println("Cannot write to config file:", err)
			return
		}

		fmt.Println("Successfully joined room !")

		return

	case PUSH:
		if config.SessionID == "" {
			fmt.Println("Please login before upload content")
			return
		}

		content := os.Args[2:]
		_, err := createNote(config, strings.Join(content, " "))
		if err != nil {
			fmt.Println("Error:", err)
			return
		}

		fmt.Println("Successfully pushed note to your session")

	case LIST:
		if config.SessionID == "" {
			fmt.Println("Please login before upload content")
			return
		}
		sessionContent, err := getSessionContent(config)
		if err != nil {
			fmt.Println("Error:", err)
			return
		}
		var options []string
		for _, content := range sessionContent {
			switch c := content.(type) {
			case NoteType:
				options = append(options, "[✎] "+truncate(c.Content, 30))
			case AttachmentType:
				options = append(options, "[⛓] "+c.AttachmentPath)
			default:
				options = append(options, "[?] Unknown Type")
			}
		}

		prompt := promptui.Select{
			Label: "Select Content",
			Items: options,
			Size:  10,
		}

		index, _, err := prompt.Run()
		if err != nil {
			fmt.Println("Selection failed:", err)
			return
		}

		fmt.Println()
		selectedContent := sessionContent[index]
		switch c := selectedContent.(type) {
		case NoteType:
			fmt.Println(c.Content)
		case AttachmentType:
			fmt.Println(c.AttachmentURL)
			err := downloadFile(c.AttachmentURL)
			if err != nil {
				fmt.Println("Download failed", err)
				return
			}
		default:
		}

	}

}
