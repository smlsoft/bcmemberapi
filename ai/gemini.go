package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type GeminiService struct {
	apiKey string
}

type geminiRequest struct {
	Contents []geminiContent `json:"contents"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type ChatMessage struct {
	Role    string
	Message string
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

func NewGeminiService(apiKey string) *GeminiService {
	return &GeminiService{apiKey: strings.TrimSpace(apiKey)}
}

func (g *GeminiService) GenerateResponse(userMessage string) (string, error) {
	return g.GenerateResponseWithHistory(userMessage, nil)
}

func (g *GeminiService) GenerateResponseWithHistory(userMessage string, history []ChatMessage) (string, error) {
	apiKey := strings.TrimSpace(g.apiKey)
	if apiKey == "" {
		return "", fmt.Errorf("gemini API key is empty")
	}
	endpoint := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-1.5-flash:generateContent?key=%s", url.QueryEscape(apiKey))

	contents := []geminiContent{}

	// Add chat history
	for _, msg := range history {
		role := "user"
		if msg.Role == "assistant" {
			role = "model"
		}
		contents = append(contents, geminiContent{
			Role: role,
			Parts: []geminiPart{
				{Text: msg.Message},
			},
		})
	}

	// Add current user message
	contents = append(contents, geminiContent{
		Role: "user",
		Parts: []geminiPart{
			{Text: userMessage},
		},
	})

	reqBody := geminiRequest{
		Contents: contents,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Post(endpoint, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gemini API error: %s", string(body))
	}

	var geminiResp geminiResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return "", err
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no response from Gemini")
	}

	return geminiResp.Candidates[0].Content.Parts[0].Text, nil
}
