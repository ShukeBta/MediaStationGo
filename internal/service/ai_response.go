package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

func parseAIResponse(protocol string, body []byte) (string, error) {
	var text string
	switch protocol {
	case "openai", "ollama":
		var out struct {
			Choices []struct{ Message ChatTurn } `json:"choices"`
			Message ChatTurn                     `json:"message"`
			Error   json.RawMessage              `json:"error"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			return "", err
		}
		if len(out.Error) > 0 && string(out.Error) != "null" && string(out.Error) != `""` {
			var message string
			if err := json.Unmarshal(out.Error, &message); err != nil {
				var detail struct{ Message string }
				_ = json.Unmarshal(out.Error, &detail)
				message = detail.Message
			}
			if strings.TrimSpace(message) == "" {
				message = "provider returned an error"
			}
			return "", fmt.Errorf("ai: %s", message)
		}
		if protocol == "ollama" {
			text = out.Message.Content
		} else if len(out.Choices) > 0 {
			text = out.Choices[0].Message.Content
		}
	case "responses":
		var out struct {
			Status string `json:"status"`
			Output []struct {
				Type    string `json:"type"`
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"output"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			return "", err
		}
		if out.Status == "failed" || out.Status == "incomplete" {
			return "", fmt.Errorf("ai: response %s", out.Status)
		}
		// Output can start with reasoning or tool calls. Collect text only
		// from message items rather than assuming output[0] contains the reply.
		for _, item := range out.Output {
			if item.Type == "message" {
				for _, part := range item.Content {
					if part.Type == "output_text" {
						text += part.Text
					}
				}
			}
		}
	case "anthropic":
		var out struct {
			Content []struct{ Type, Text string } `json:"content"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			return "", err
		}
		for _, part := range out.Content {
			if part.Type == "text" {
				text += part.Text
			}
		}
	case "gemini":
		var out struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text    string `json:"text"`
						Thought bool   `json:"thought"`
					} `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			return "", err
		}
		if len(out.Candidates) > 0 {
			for _, part := range out.Candidates[0].Content.Parts {
				if !part.Thought {
					text += part.Text
				}
			}
		}
	default:
		return "", fmt.Errorf("unsupported AI protocol: %s", protocol)
	}
	if text = strings.TrimSpace(text); text == "" {
		return "", errors.New("ai: empty completion")
	}
	return text, nil
}
