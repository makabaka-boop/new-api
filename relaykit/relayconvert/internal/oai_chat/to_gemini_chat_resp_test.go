package oaichat

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponseOpenAI2GeminiMapsTextToolFinishReasonAndUsage(t *testing.T) {
	msg := dto.Message{
		Role:    "assistant",
		Content: "hello",
	}
	msg.SetToolCalls([]dto.ToolCallRequest{
		{
			ID:   "call_1",
			Type: "function",
			Function: dto.FunctionRequest{
				Name:      "lookup",
				Arguments: `{"q":"x"}`,
			},
		},
	})

	resp := ResponseOpenAI2Gemini(&dto.OpenAITextResponse{
		Model: "gpt-test",
		Choices: []dto.OpenAITextResponseChoice{
			{
				Index:        2,
				Message:      msg,
				FinishReason: "length",
			},
		},
		Usage: dto.Usage{
			PromptTokens:     11,
			CompletionTokens: 5,
			TotalTokens:      16,
		},
	}, nil)

	assert.Equal(t, 11, resp.UsageMetadata.PromptTokenCount)
	assert.Equal(t, 5, resp.UsageMetadata.CandidatesTokenCount)
	assert.Equal(t, 16, resp.UsageMetadata.TotalTokenCount)
	require.NotNil(t, resp.UsageMetadata.BillingUsage)
	require.NotNil(t, resp.UsageMetadata.BillingUsage.OpenAIUsage)
	assert.Equal(t, dto.BillingUsageSourceOAIChat, resp.UsageMetadata.BillingUsage.Source)
	assert.Equal(t, dto.BillingUsageSemanticOpenAI, resp.UsageMetadata.BillingUsage.Semantic)
	assert.Equal(t, 11, resp.UsageMetadata.BillingUsage.OpenAIUsage.PromptTokens)
	assert.Equal(t, 5, resp.UsageMetadata.BillingUsage.OpenAIUsage.CompletionTokens)
	assert.Equal(t, 16, resp.UsageMetadata.BillingUsage.OpenAIUsage.TotalTokens)
	assert.Nil(t, resp.UsageMetadata.BillingUsage.OpenAIUsage.BillingUsage)
	require.Len(t, resp.Candidates, 1)
	assert.Equal(t, int64(2), resp.Candidates[0].Index)
	require.NotNil(t, resp.Candidates[0].FinishReason)
	assert.Equal(t, "MAX_TOKENS", *resp.Candidates[0].FinishReason)
	require.Len(t, resp.Candidates[0].Content.Parts, 2)
	assert.Equal(t, "hello", resp.Candidates[0].Content.Parts[0].Text)
	require.NotNil(t, resp.Candidates[0].Content.Parts[1].FunctionCall)
	assert.Equal(t, "lookup", resp.Candidates[0].Content.Parts[1].FunctionCall.FunctionName)
	assert.Equal(t, map[string]any{"q": "x"}, resp.Candidates[0].Content.Parts[1].FunctionCall.Arguments)
}

func TestStreamResponseOpenAI2GeminiMapsToolCallFinishReasonAndUsage(t *testing.T) {
	resp := StreamResponseOpenAI2Gemini(&dto.ChatCompletionsStreamResponse{
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{
				Index:        1,
				FinishReason: geminiRespPtr("tool_calls"),
				Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
					ToolCalls: []dto.ToolCallResponse{
						{
							Type: "function",
							Function: dto.FunctionResponse{
								Name:      "lookup",
								Arguments: `{"q":"x"}`,
							},
						},
					},
				},
			},
		},
		Usage: &dto.Usage{
			PromptTokens:     13,
			CompletionTokens: 8,
			TotalTokens:      21,
		},
	}, &convmeta.Values{})

	require.NotNil(t, resp)
	assert.Equal(t, 13, resp.UsageMetadata.PromptTokenCount)
	assert.Equal(t, 8, resp.UsageMetadata.CandidatesTokenCount)
	assert.Equal(t, 21, resp.UsageMetadata.TotalTokenCount)
	require.NotNil(t, resp.UsageMetadata.BillingUsage)
	require.NotNil(t, resp.UsageMetadata.BillingUsage.OpenAIUsage)
	assert.Equal(t, 13, resp.UsageMetadata.BillingUsage.OpenAIUsage.PromptTokens)
	assert.Equal(t, 8, resp.UsageMetadata.BillingUsage.OpenAIUsage.CompletionTokens)
	require.Len(t, resp.Candidates, 1)
	assert.Equal(t, int64(1), resp.Candidates[0].Index)
	require.NotNil(t, resp.Candidates[0].FinishReason)
	assert.Equal(t, "STOP", *resp.Candidates[0].FinishReason)
	require.Len(t, resp.Candidates[0].Content.Parts, 1)
	require.NotNil(t, resp.Candidates[0].Content.Parts[0].FunctionCall)
	assert.Equal(t, "lookup", resp.Candidates[0].Content.Parts[0].FunctionCall.FunctionName)
	assert.Equal(t, map[string]any{"q": "x"}, resp.Candidates[0].Content.Parts[0].FunctionCall.Arguments)
}

func geminiRespPtr[T any](value T) *T {
	return &value
}

const (
	geminiCitA = `[{"type":"url_citation","url_citation":{"url":"https://a.example/","title":"A","start_index":0,"end_index":5}}]`
	geminiCitB = `[{"type":"url_citation","url_citation":{"url":"https://b.example/","title":"B","start_index":6,"end_index":11}}]`
)

func geminiCitationRange(t *testing.T, text string, citation dto.GeminiCitation) string {
	t.Helper()
	runes := []rune(text)
	require.NotNil(t, citation.StartIndex)
	require.NotNil(t, citation.EndIndex)
	return string(runes[*citation.StartIndex:*citation.EndIndex])
}

func TestResponseOpenAI2GeminiMapsURLCitations(t *testing.T) {
	message := dto.Message{Role: "assistant", Content: "hello world"}
	message.Annotations = json.RawMessage(`[
		{"type":"url_citation","url_citation":{"url":"https://a.example/","title":"A","start_index":0,"end_index":5}},
		{"type":"url_citation","url_citation":{"url":"https://a.example/","title":"A","start_index":0,"end_index":5}},
		{"type":"url_citation","url_citation":{"url":"https://b.example/","title":"B","start_index":6,"end_index":11}},
		{"type":"url_citation","url_citation":{"url":"https://oob.example/","title":"OOB","start_index":0,"end_index":99}},
		{"type":"url_citation","url_citation":{"url":"","start_index":0,"end_index":1}},
		{"type":"web_search","url":"https://ignored.example/"},
		{"type":"url_citation","url":"https://flat.example/","start_index":0,"end_index":5}
	]`)

	other := dto.Message{Role: "assistant", Content: "你好，世界"}
	other.Annotations = json.RawMessage(`[
		{"type":"url_citation","url_citation":{"url":"https://c.example/","title":"C","start_index":0,"end_index":2}}
	]`)

	resp := ResponseOpenAI2Gemini(&dto.OpenAITextResponse{
		Choices: []dto.OpenAITextResponseChoice{
			{Index: 0, Message: message, FinishReason: "stop"},
			{Index: 1, Message: other, FinishReason: "stop"},
		},
	}, nil)

	require.Len(t, resp.Candidates, 2)

	require.NotNil(t, resp.Candidates[0].CitationMetadata)
	citations := resp.Candidates[0].CitationMetadata.Citations
	require.Len(t, citations, 3)
	assert.Equal(t, "https://a.example/", citations[0].URI)
	assert.Equal(t, "A", citations[0].Title)
	assert.Equal(t, "hello", geminiCitationRange(t, "hello world", citations[0]))
	assert.Equal(t, "https://b.example/", citations[1].URI)
	assert.Equal(t, "world", geminiCitationRange(t, "hello world", citations[1]))
	assert.Equal(t, "https://flat.example/", citations[2].URI)
	assert.Equal(t, "hello", geminiCitationRange(t, "hello world", citations[2]))

	require.NotNil(t, resp.Candidates[1].CitationMetadata)
	require.Len(t, resp.Candidates[1].CitationMetadata.Citations, 1)
	assert.Equal(t, "https://c.example/", resp.Candidates[1].CitationMetadata.Citations[0].URI)
	assert.Equal(t, "你好", geminiCitationRange(t, "你好，世界", resp.Candidates[1].CitationMetadata.Citations[0]))
}

func geminiChatStreamChunk(choiceIndex int, text string, finishReason string, annotations string) *dto.ChatCompletionsStreamResponse {
	choice := dto.ChatCompletionsStreamResponseChoice{Index: choiceIndex}
	if text != "" {
		choice.Delta.SetContentString(text)
	}
	if finishReason != "" {
		choice.FinishReason = &finishReason
	}
	if annotations != "" {
		choice.Delta.Annotations = json.RawMessage(annotations)
	}
	return &dto.ChatCompletionsStreamResponse{Choices: []dto.ChatCompletionsStreamResponseChoice{choice}}
}

func convertGeminiChatChunk(t *testing.T, state *ChatToGeminiStreamState, chunk *dto.ChatCompletionsStreamResponse) []*dto.GeminiChatResponse {
	t.Helper()
	responses, err := state.ConvertChunk(chunk, &convmeta.Values{})
	require.NoError(t, err)
	return responses
}

func collectGeminiCitationResponses(t *testing.T, state *ChatToGeminiStreamState, chunks []*dto.ChatCompletionsStreamResponse) []*dto.GeminiChatResponse {
	t.Helper()
	var all []*dto.GeminiChatResponse
	for _, chunk := range chunks {
		all = append(all, convertGeminiChatChunk(t, state, chunk)...)
	}
	return all
}

func collectGeminiStreamCandidates(responses []*dto.GeminiChatResponse) []dto.GeminiChatCandidate {
	var candidates []dto.GeminiChatCandidate
	for _, response := range responses {
		candidates = append(candidates, response.Candidates...)
	}
	return candidates
}

func TestChatToGeminiStreamCitationsArriveAfterBody(t *testing.T) {
	state := NewChatToGeminiStreamState()
	responses := collectGeminiCitationResponses(t, state, []*dto.ChatCompletionsStreamResponse{
		geminiChatStreamChunk(0, "hello ", "", ""),
		geminiChatStreamChunk(0, "world", "", ""),
		geminiChatStreamChunk(0, "", "", geminiCitA),
		geminiChatStreamChunk(0, "", "stop", ""),
	})

	candidates := collectGeminiStreamCandidates(responses)
	require.Len(t, candidates, 4)
	assert.Equal(t, "hello ", candidates[0].Content.Parts[0].Text)
	assert.Nil(t, candidates[0].CitationMetadata)
	assert.Equal(t, "world", candidates[1].Content.Parts[0].Text)
	assert.Nil(t, candidates[1].CitationMetadata)

	require.NotNil(t, candidates[2].CitationMetadata)
	require.Len(t, candidates[2].CitationMetadata.Citations, 1)
	citation := candidates[2].CitationMetadata.Citations[0]
	assert.Equal(t, "https://a.example/", citation.URI)
	assert.Equal(t, "A", citation.Title)
	assert.Equal(t, "hello", geminiCitationRange(t, "hello world", citation))
	assert.Empty(t, candidates[2].Content.Parts)
	assert.Nil(t, candidates[2].FinishReason)

	require.NotNil(t, candidates[3].FinishReason)
	assert.Equal(t, "STOP", *candidates[3].FinishReason)
	assert.Nil(t, candidates[3].CitationMetadata)
}

func TestChatToGeminiStreamCitationsSpanFragments(t *testing.T) {
	state := NewChatToGeminiStreamState()
	responses := collectGeminiCitationResponses(t, state, []*dto.ChatCompletionsStreamResponse{
		geminiChatStreamChunk(0, "hel", "", geminiCitA),
		geminiChatStreamChunk(0, "lo", "", ""),
		geminiChatStreamChunk(0, " world", "", ""),
		geminiChatStreamChunk(0, "", "stop", ""),
	})

	candidates := collectGeminiStreamCandidates(responses)
	require.Len(t, candidates, 4)
	// The citation spans text that had not arrived when the annotation did,
	// so the first chunk carries only its text fragment.
	assert.Equal(t, "hel", candidates[0].Content.Parts[0].Text)
	assert.Nil(t, candidates[0].CitationMetadata)
	// Once the cited range is complete the citation is attached to a later
	// fragment while still pointing at the absolute answer range.
	assert.Equal(t, "lo", candidates[1].Content.Parts[0].Text)
	require.NotNil(t, candidates[1].CitationMetadata)
	require.Len(t, candidates[1].CitationMetadata.Citations, 1)
	assert.Equal(t, "hello", geminiCitationRange(t, "hello world", candidates[1].CitationMetadata.Citations[0]))
	assert.Nil(t, candidates[2].CitationMetadata)
	assert.Nil(t, candidates[3].CitationMetadata)
}

func TestChatToGeminiStreamCitationDedupeAndUnlocatable(t *testing.T) {
	state := NewChatToGeminiStreamState()
	first := convertGeminiChatChunk(t, state, geminiChatStreamChunk(0, "hello world", "", geminiCitB))
	require.Len(t, first, 1)
	require.NotNil(t, first[0].Candidates[0].CitationMetadata)
	require.Len(t, first[0].Candidates[0].CitationMetadata.Citations, 1)
	assert.Equal(t, "world", geminiCitationRange(t, "hello world", first[0].Candidates[0].CitationMetadata.Citations[0]))

	// The same uri+range arrives again: it is kept once and produces no chunk.
	assert.Nil(t, convertGeminiChatChunk(t, state, geminiChatStreamChunk(0, "", "", geminiCitB)))

	// A range beyond the final text is buffered while the choice is open.
	assert.Nil(t, convertGeminiChatChunk(t, state, geminiChatStreamChunk(0, "", "", `[
		{"type":"url_citation","url_citation":{"url":"https://oob.example/","start_index":0,"end_index":99}}
	]`)))

	finishResponses := convertGeminiChatChunk(t, state, geminiChatStreamChunk(0, "", "stop", ""))
	require.Len(t, finishResponses, 1)
	finishCandidate := finishResponses[0].Candidates[0]
	require.NotNil(t, finishCandidate.FinishReason)
	assert.Nil(t, finishCandidate.CitationMetadata)

	finalized, err := state.Finalize(&convmeta.Values{})
	require.NoError(t, err)
	assert.Empty(t, finalized)
}

func TestChatToGeminiStreamCitationsAfterFinish(t *testing.T) {
	state := NewChatToGeminiStreamState()
	responses := collectGeminiCitationResponses(t, state, []*dto.ChatCompletionsStreamResponse{
		geminiChatStreamChunk(0, "hello world", "", ""),
		geminiChatStreamChunk(0, "", "stop", ""),
		geminiChatStreamChunk(0, "", "", `[
			{"type":"url_citation","url_citation":{"url":"https://a.example/","title":"A","start_index":0,"end_index":5}},
			{"type":"url_citation","url_citation":{"url":"https://oob.example/","start_index":0,"end_index":99}}
		]`),
	})

	candidates := collectGeminiStreamCandidates(responses)
	require.Len(t, candidates, 3)
	require.NotNil(t, candidates[2].CitationMetadata)
	require.Len(t, candidates[2].CitationMetadata.Citations, 1)
	citation := candidates[2].CitationMetadata.Citations[0]
	assert.Equal(t, "https://a.example/", citation.URI)
	assert.Equal(t, "hello", geminiCitationRange(t, "hello world", citation))
	assert.Nil(t, candidates[2].FinishReason)
	assert.Empty(t, candidates[2].Content.Parts)
}

func TestChatToGeminiStreamCitationFinalizeDropsUnlocatable(t *testing.T) {
	state := NewChatToGeminiStreamState()
	responses := convertGeminiChatChunk(t, state, geminiChatStreamChunk(0, "hello world", "", `[
		{"type":"url_citation","url_citation":{"url":"https://oob.example/","start_index":0,"end_index":99}}
	]`))
	require.Len(t, responses, 1)
	assert.Nil(t, responses[0].Candidates[0].CitationMetadata)

	// Upstream closes without a finish-reason chunk: the cited range never
	// arrives, so the citation is skipped but the answer is finalized.
	finalized, err := state.Finalize(&convmeta.Values{})
	require.NoError(t, err)
	require.Len(t, finalized, 1)
	require.Len(t, finalized[0].Candidates, 1)
	assert.Nil(t, finalized[0].Candidates[0].CitationMetadata)
	require.NotNil(t, finalized[0].Candidates[0].FinishReason)
	assert.Equal(t, "STOP", *finalized[0].Candidates[0].FinishReason)
}

func TestChatToGeminiStreamCitationsAttachToOwnChoice(t *testing.T) {
	state := NewChatToGeminiStreamState()
	convertGeminiChatChunk(t, state, geminiChatStreamChunk(0, "aaa", "", ""))
	convertGeminiChatChunk(t, state, geminiChatStreamChunk(1, "bbb", "", ""))

	responses, err := state.ConvertChunk(&dto.ChatCompletionsStreamResponse{
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{
				Index: 0,
				Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
					Annotations: json.RawMessage(`[{"type":"url_citation","url_citation":{"url":"https://a.example/","start_index":0,"end_index":3}}]`),
				},
			},
			{
				Index: 1,
				Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
					Annotations: json.RawMessage(`[{"type":"url_citation","url_citation":{"url":"https://b.example/","start_index":0,"end_index":3}}]`),
				},
			},
		},
	}, &convmeta.Values{})
	require.NoError(t, err)
	require.Len(t, responses, 1)
	require.Len(t, responses[0].Candidates, 2)
	assert.Equal(t, int64(0), responses[0].Candidates[0].Index)
	require.NotNil(t, responses[0].Candidates[0].CitationMetadata)
	assert.Equal(t, "https://a.example/", responses[0].Candidates[0].CitationMetadata.Citations[0].URI)
	assert.Equal(t, "aaa", geminiCitationRange(t, "aaa", responses[0].Candidates[0].CitationMetadata.Citations[0]))
	assert.Equal(t, int64(1), responses[0].Candidates[1].Index)
	require.NotNil(t, responses[0].Candidates[1].CitationMetadata)
	assert.Equal(t, "https://b.example/", responses[0].Candidates[1].CitationMetadata.Citations[0].URI)
	assert.Equal(t, "bbb", geminiCitationRange(t, "bbb", responses[0].Candidates[1].CitationMetadata.Citations[0]))
}
