package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const subtitleTranslationMaxContext = 5

type SubtitleTranslationInput struct {
	Text             string   `json:"text"`
	Context          []string `json:"context"`
	Glossary         string   `json:"glossary"`
	RetryInstruction string   `json:"retry_instruction"`
}

type SubtitleTranslationResult struct {
	Translation string `json:"translation"`
}

func (s *SubtitleService) TranslateText(ctx context.Context, provider, selectedModel string, input SubtitleTranslationInput) (SubtitleTranslationResult, error) {
	if s == nil || s.apiConfig == nil {
		return SubtitleTranslationResult{}, errors.New("AI translation configuration unavailable")
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider != "openai" && provider != "deepseek" && provider != "siliconflow" {
		return SubtitleTranslationResult{}, errors.New("unsupported cloud translation provider")
	}
	selectedModel = strings.TrimSpace(selectedModel)
	resolved, err := s.apiConfig.Resolve(ctx, provider)
	if err != nil {
		return SubtitleTranslationResult{}, err
	}
	if !resolved.Enabled || strings.TrimSpace(resolved.APIKey) == "" || strings.TrimSpace(resolved.BaseURL) == "" {
		return SubtitleTranslationResult{}, errors.New("cloud translation provider is not configured or enabled")
	}
	if selectedModel == "" || selectedModel != strings.TrimSpace(resolved.Model) {
		return SubtitleTranslationResult{}, errors.New("selected cloud translation model does not match the configured model")
	}
	validated, err := validateSubtitleTranslationInput(input)
	if err != nil {
		return SubtitleTranslationResult{}, err
	}
	payload := map[string]any{
		"model":       selectedModel,
		"max_tokens":  1024,
		"temperature": 0.1,
		"top_p":       0.9,
		"stream":      false,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": subtitleTranslationPrompt(validated),
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return SubtitleTranslationResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, aiChatCompletionsEndpoint(resolved.BaseURL), bytes.NewReader(body))
	if err != nil {
		return SubtitleTranslationResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+resolved.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := NewExternalHTTPClient(120 * time.Second).Do(req)
	if err != nil {
		return SubtitleTranslationResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		return SubtitleTranslationResult{}, fmt.Errorf("cloud translation failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var upstream struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 32*1024*1024))
	if err := decoder.Decode(&upstream); err != nil {
		return SubtitleTranslationResult{}, fmt.Errorf("cloud translation response is invalid: %w", err)
	}
	if len(upstream.Choices) == 0 {
		return SubtitleTranslationResult{}, errors.New("cloud translation returned no choices")
	}
	translation, err := validateSubtitleTranslationOutput(validated.Text, upstream.Choices[0].Message.Content)
	if err != nil {
		return SubtitleTranslationResult{}, err
	}
	return SubtitleTranslationResult{Translation: translation}, nil
}

func validateSubtitleTranslationInput(input SubtitleTranslationInput) (SubtitleTranslationInput, error) {
	input.Text = strings.TrimSpace(input.Text)
	input.Glossary = strings.TrimSpace(input.Glossary)
	input.RetryInstruction = strings.TrimSpace(input.RetryInstruction)
	if input.Text == "" {
		return SubtitleTranslationInput{}, errors.New("translation target text is required")
	}
	if len(input.Context) > subtitleTranslationMaxContext {
		return SubtitleTranslationInput{}, fmt.Errorf("translation context cannot exceed %d segments", subtitleTranslationMaxContext)
	}
	if len([]rune(input.RetryInstruction)) > 200 {
		return SubtitleTranslationInput{}, errors.New("translation retry instruction exceeds the text limit")
	}
	totalChars := len([]rune(input.Text)) + len([]rune(input.Glossary)) + len([]rune(input.RetryInstruction))
	for i := range input.Context {
		input.Context[i] = strings.TrimSpace(input.Context[i])
		if input.Context[i] == "" {
			return SubtitleTranslationInput{}, errors.New("translation context contains empty text")
		}
		totalChars += len([]rune(input.Context[i]))
	}
	if totalChars > 5000 {
		return SubtitleTranslationInput{}, errors.New("translation request exceeds the text limit")
	}
	return input, nil
}

func validateSubtitleTranslationOutput(source, translated string) (string, error) {
	translated = strings.TrimSpace(translated)
	if translated == "" {
		return "", errors.New("cloud translation returned empty content")
	}
	if strings.Contains(translated, "```") || strings.HasPrefix(translated, "{") || strings.HasPrefix(translated, "[") {
		return "", errors.New("cloud translation returned structured content instead of plain text")
	}
	if strings.Contains(strings.ToLower(translated), "<think") || strings.Contains(strings.ToLower(translated), "</think>") {
		return "", errors.New("cloud translation returned reasoning content instead of plain text")
	}
	for _, prefix := range []string{"译文：", "翻译：", "翻译结果：", "Translation:"} {
		if strings.HasPrefix(translated, prefix) {
			return "", errors.New("cloud translation returned an explanation instead of plain text")
		}
	}
	if strings.EqualFold(strings.Join(strings.Fields(source), ""), strings.Join(strings.Fields(translated), "")) {
		return "", errors.New("cloud translation returned the untranslated source text")
	}
	if containsSubtitleTranslationJapaneseKana(translated) {
		return "", errors.New("cloud translation still contains Japanese kana")
	}
	sourceLength := len([]rune(source))
	translatedLength := len([]rune(translated))
	maxLength := sourceLength*4 + 20
	if maxLength < 80 {
		maxLength = 80
	}
	if translatedLength > maxLength {
		return "", errors.New("cloud translation length is abnormally large")
	}
	if sourceLength >= 20 && translatedLength < sourceLength/10 {
		return "", errors.New("cloud translation length is abnormally small")
	}
	return translated, nil
}

func containsSubtitleTranslationJapaneseKana(value string) bool {
	for _, char := range value {
		if (char >= '\u3040' && char <= '\u30ff') || (char >= '\u31f0' && char <= '\u31ff') {
			return true
		}
	}
	return false
}

func subtitleTranslationPrompt(input SubtitleTranslationInput) string {
	contextText := "（无）"
	if len(input.Context) > 0 {
		contextText = strings.Join(input.Context, "\n")
	}
	glossaryText := "（无）"
	if input.Glossary != "" {
		glossaryText = input.Glossary
	}
	retryText := ""
	if input.RetryInstruction != "" {
		retryText = "\n\n重试要求：\n" + input.RetryInstruction
	}
	return "参考上下文：\n" + contextText +
		"\n\n术语参考：\n" + glossaryText +
		retryText +
		"\n\n将下面的日文翻译成自然、准确的简体中文。\n只输出译文，不要解释：\n\n" + input.Text
}

func aiChatCompletionsEndpoint(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	baseURL = strings.TrimSuffix(baseURL, "/chat/completions")
	return strings.TrimRight(baseURL, "/") + "/chat/completions"
}
