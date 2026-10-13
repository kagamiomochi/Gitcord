package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Shared client with a timeout so a slow network never hangs the commit flow
var trClient = &http.Client{Timeout: 10 * time.Second}

// trConfig is the translation setting sent from the settings dialog.
// Empty fields fall back to the environment variables.
type trConfig struct {
	Engine        string // "auto" (default), "deepl" or "mymemory"
	DeepLKey      string
	MyMemoryEmail string
}

// resolve fills empty fields from the environment and trims whitespace.
func (c trConfig) resolve() trConfig {
	c.Engine = strings.TrimSpace(c.Engine)
	c.DeepLKey = strings.TrimSpace(c.DeepLKey)
	c.MyMemoryEmail = strings.TrimSpace(c.MyMemoryEmail)
	if c.DeepLKey == "" {
		c.DeepLKey = strings.TrimSpace(os.Getenv("GITCORD_DEEPL_KEY"))
	}
	if c.MyMemoryEmail == "" {
		c.MyMemoryEmail = strings.TrimSpace(os.Getenv("GITCORD_MYMEMORY_EMAIL"))
	}
	return c
}

// pickEngine returns "deepl" or "mymemory" for an already resolved config.
func pickEngine(c trConfig) (string, error) {
	switch c.Engine {
	case "deepl":
		if c.DeepLKey == "" {
			return "", errors.New("DeepLのAPIキーが設定されていません")
		}
		return "deepl", nil
	case "mymemory":
		return "mymemory", nil
	case "", "auto":
		if c.DeepLKey != "" {
			return "deepl", nil
		}
		return "mymemory", nil
	}
	return "", errors.New("不明な翻訳エンジンです")
}

// deeplEndpoint picks the API host by key type: free-plan keys end with ":fx"
func deeplEndpoint(key string) string {
	if strings.HasSuffix(key, ":fx") {
		return "https://api-free.deepl.com/v2/translate"
	}
	return "https://api.deepl.com/v2/translate"
}

// translate uses the engine chosen in the settings (auto: DeepL when a key exists, else MyMemory)
func translate(text string, cfg trConfig) (string, error) {
	cfg = cfg.resolve()
	engine, err := pickEngine(cfg)
	if err != nil {
		return "", err
	}
	if engine == "deepl" {
		return translateDeepL(text, cfg.DeepLKey)
	}
	return translateMyMemory(text, cfg.MyMemoryEmail)
}

func translateDeepL(text, key string) (string, error) {
	form := url.Values{"text": {text}, "target_lang": {"EN"}}
	req, err := http.NewRequest("POST", deeplEndpoint(key), strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "DeepL-Auth-Key "+key)
	resp, err := trClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden {
		return "", errors.New("DeepLのAPIキーが正しくありません")
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("DeepLがHTTP %dを返しました", resp.StatusCode)
	}
	var out struct {
		Translations []struct {
			Text string `json:"text"`
		} `json:"translations"`
	}
	if json.NewDecoder(resp.Body).Decode(&out) != nil || len(out.Translations) == 0 {
		return "", errors.New("DeepLの応答を解析できませんでした")
	}
	return strings.TrimSpace(out.Translations[0].Text), nil
}

// translateMyMemory translates line by line to stay under the per-request size limit
func translateMyMemory(text, mail string) (string, error) {
	lines := strings.Split(text, "\n")
	for i, ln := range lines {
		// Skip empty or ASCII-only lines, there is nothing to translate
		if strings.TrimSpace(ln) == "" || !hasNonASCII(ln) {
			continue
		}
		t, err := myMemoryLine(ln, mail)
		if err != nil {
			return "", err
		}
		lines[i] = t
	}
	return strings.TrimSpace(strings.Join(lines, "\n")), nil
}

func hasNonASCII(s string) bool {
	for _, r := range s {
		if r > 0x7f {
			return true
		}
	}
	return false
}

func myMemoryLine(s, mail string) (string, error) {
	q := url.Values{"q": {s}, "langpair": {"ja|en"}}
	// Optional contact email raises the daily quota from 5k to 50k chars
	if mail != "" {
		q.Set("de", mail)
	}
	resp, err := trClient.Get("https://api.mymemory.translated.net/get?" + q.Encode())
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("MyMemoryがHTTP %dを返しました", resp.StatusCode)
	}
	var out struct {
		Data struct {
			Text string `json:"translatedText"`
		} `json:"responseData"`
	}
	if json.NewDecoder(resp.Body).Decode(&out) != nil || out.Data.Text == "" {
		return "", errors.New("MyMemoryの応答を解析できませんでした")
	}
	// MyMemory reports quota exhaustion inside a normal 200 response body
	if strings.HasPrefix(out.Data.Text, "MYMEMORY WARNING") {
		return "", errors.New("MyMemoryの無料枠を使い切りました")
	}
	return out.Data.Text, nil
}
