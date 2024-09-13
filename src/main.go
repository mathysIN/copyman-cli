package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"reflect"
	"strings"
)

const configFolderPath = "/copyman/"
const configFolderFileName = "config"

type Post struct {
	SessionID        string `json:"sessionId"`
	CreateNewSession bool   `json:"createNewSession"`
}

type KeyValue struct {
	Key   string
	Value string
}

type Config struct {
	Token string
}

type Command string

const (
	NONE   Command = "none"
	LOGIN   Command = "login"
	LOGOUT  Command = "logout"
	CONTENT Command = "content"
)

func getJSON(url string, target interface{}) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	return json.Unmarshal(body, target)
}

func postJSON(url string, target interface{}, note string) (*http.Response, error) {

	body := []byte(`{
		"content":"` + note + `"
	}`)

	sessionCookie := &http.Cookie{
		Name:  "session",
		Value: "mathys",
	}

	passwordCookie := &http.Cookie{
		Name:  "password",
		Value: "b712010b801c6d2265fe3d6b05911c660946476fc3185be8f14be503cf45b5d27f6c33baa6a8ceca4f7c4b49bf54c0cf8046c0e8153fcff71edf5afa9e66b3cd",
	}

	r, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	jar, _ := cookiejar.New(nil)
	client := http.Client{
		Jar: jar,
	}

	client.Jar.SetCookies(r.URL, []*http.Cookie{sessionCookie, passwordCookie})

	if err != nil {
		return nil, err
	}

	response, err := client.Do(r)

	return response, nil
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

	if err != nil {
		return err
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
		case "Token":
			config.Token = keyVal.Value
		default:
			fmt.Printf("Unknown config key: %s\n", keyVal.Key)
		}
	}

	return config, nil
}

func main() {
	url := "https://copyman.fr/api/notes"
	var post Post

	contentType := os.Args[1]

	var command Command = NONE

	switch strings.ToLower(contentType) {
	case "login":
		command = LOGIN
	case "logout":
		command = LOGOUT
		break
	case "push":
		command = CONTENT
		break
	}

	if command == NONE {
		return
	}

	content := os.Args[2:]

	response, err := postJSON(url, &post, strings.Join(content, " "))
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Println(response)

	fmt.Printf("Response: %+v\n", post)

	createDefaultConfig()

	config, err := getConfig()

	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	config.Token = config.Token + "2"

	writeConfig(config)

	fmt.Println(config)
}
