package ApiInterface

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/joho/godotenv"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

var storage sync.Map

const (
	// The key used to store the token in the storage
	tokenKey = "hasura-jwt-token"
)

func base64urlUnescape(str string) string {
	str = strings.ReplaceAll(str, "-", "+")
	str = strings.ReplaceAll(str, "_", "/")
	switch len(str) % 4 {
	case 2:
		str += "=="
	case 3:
		str += "="
	}
	return str
}

func Decode(token string) (map[string]interface{}, error) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil, errors.New("invalid token")
	}
	decoded, err := base64.StdEncoding.DecodeString(base64urlUnescape(parts[1]))
	if err != nil {
		return nil, err
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return nil, err
	}

	return payload, nil
}

func fetch(domain, path string, headers map[string]string, data []byte) ([]byte, error) {
	client := &http.Client{}
	method := "GET"
	if data != nil {
		method = "POST"
	}

	req, err := http.NewRequest(method, fmt.Sprintf("https://%s%s", domain, path), bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(resp.Status)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
}

func isExpired(payload map[string]interface{}) bool {
	exp, ok := payload["exp"].(float64)
	if !ok {
		return true
	}
	diff := exp - float64(time.Now().Unix())
	return diff <= 0
}

func refreshToken(domain, token string) (string, map[string]interface{}, error) {
	headers := map[string]string{
		"x-jwt-token": token,
	}
	res, err := fetch(domain, "/api/auth/refresh", headers, nil)
	if err != nil {
		return "", nil, err
	}

	newToken := string(res)
	payload, err := Decode(newToken)
	if err != nil {
		return "", nil, err
	}

	return newToken, payload, nil
}

func LoadToken() string {
	// open the .env file
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	return os.Getenv("TOKEN")
}

func StoreToken(token string) {
	// open the .env file
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	os.Setenv("TOKEN", token)
	file, err := os.OpenFile(".env", os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	_, err = file.WriteString("TOKEN=" + token)
	if err != nil {
		log.Fatal(err)
	}
}

type Client struct {
	domain            string
	accessToken       string
	mu                sync.Mutex
	pendingTokenQuery *sync.Once
}

func NewClient(domain string) (*Client, error) {
	client := &Client{
		domain: domain,
	}
	InitialJWT := LoadToken()
	refreshedToken, _, err := refreshToken(domain, InitialJWT)
	//remove the first and last character of the string
	if err != nil {
		return nil, err
	}
	storage.Store(tokenKey, refreshedToken[1:len(refreshedToken)-1])
	StoreToken(refreshedToken[1 : len(refreshedToken)-1])

	client.pendingTokenQuery = &sync.Once{}

	return client, nil
}

func (c *Client) getToken() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	val, ok := storage.Load(tokenKey)
	JWT := val.(string)
	if !ok {
		return "", errors.New("token not found")
	}
	payload, err := Decode(JWT)
	if err != nil {
		return "", err
	}
	if isExpired(payload) {
		var err error
		JWT, payload, err = refreshToken(c.domain, JWT)
		if err != nil {
			return "", err
		}
	}

	return JWT, nil
}

func (c *Client) Run(query string, variables map[string]interface{}) (map[string]interface{}, error) {
	form, err := json.Marshal(map[string]interface{}{
		"query":     query,
		"variables": variables,
	})
	if err != nil {
		return nil, err
	}

	token, err := c.getToken()
	if err != nil {
		return nil, err
	}

	headers := map[string]string{
		"Authorization":  "Bearer " + token,
		"Content-Type":   "application/json",
		"Content-Length": fmt.Sprintf("%d", len(form)),
	}

	body, err := fetch(c.domain, "/api/graphql-engine/v1/graphql", headers, form)
	if err != nil {
		return nil, err
	}

	var response struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Data map[string]interface{} `json:"data"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	if len(response.Errors) > 0 {
		return nil, errors.New(response.Errors[0].Message)
	}

	return response.Data, nil
}
