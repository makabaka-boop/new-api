package oaichat

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/relaykit/dto"
	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
)

func chatAnnotationsToClaude(raw json.RawMessage, text string) []json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	var annotations []map[string]any
	if err := kitutil.Unmarshal(raw, &annotations); err != nil {
		return nil
	}
	citations := make([]json.RawMessage, 0, len(annotations))
	for _, annotation := range annotations {
		if strings.TrimSpace(kitutil.Interface2String(annotation["type"])) != "url_citation" {
			continue
		}
		citation, ok := annotation["url_citation"].(map[string]any)
		if !ok {
			citation = annotation
		}
		url := strings.TrimSpace(kitutil.Interface2String(citation["url"]))
		if url == "" {
			continue
		}
		converted := map[string]any{
			"type":  "web_search_result_location",
			"url":   url,
			"title": strings.TrimSpace(kitutil.Interface2String(citation["title"])),
		}
		if citedText := kitutil.Interface2String(citation["cited_text"]); citedText != "" {
			converted["cited_text"] = citedText
		} else if citedText := citedTextFromAnnotation(text, citation); citedText != "" {
			converted["cited_text"] = citedText
		}
		if encryptedIndex := kitutil.Interface2String(citation["encrypted_index"]); encryptedIndex != "" {
			converted["encrypted_index"] = encryptedIndex
		}
		if converted["title"] == "" {
			delete(converted, "title")
		}
		encoded, err := kitutil.Marshal(converted)
		if err == nil {
			citations = append(citations, encoded)
		}
	}
	return citations
}

func citedTextFromAnnotation(text string, citation map[string]any) string {
	start, startOK := annotationIndex(citation["start_index"])
	end, endOK := annotationIndex(citation["end_index"])
	if !startOK || !endOK || start < 0 || end <= start {
		return ""
	}
	if end > utf8.RuneCountInString(text) {
		return ""
	}
	runes := []rune(text)
	return string(runes[start:end])
}

func annotationIndex(value any) (int, bool) {
	switch number := value.(type) {
	case float64:
		return int(number), number >= 0 && number == float64(int(number))
	case int:
		return number, number >= 0
	case json.Number:
		parsed, err := number.Int64()
		return int(parsed), err == nil && parsed >= 0
	default:
		return 0, false
	}
}

// chatGeminiCitation is the parsed form of an OpenAI Chat url_citation
// annotation. Offsets are absolute Unicode code-point indices into the
// complete answer text, exactly like upstream start_index/end_index.
type chatGeminiCitation struct {
	uri   string
	title string
	start int
	end   int
}

func chatGeminiCitationKey(c chatGeminiCitation) string {
	return fmt.Sprintf("%s\x00%d\x00%d", c.uri, c.start, c.end)
}

// parseChatURLCitations extracts structurally valid url_citation annotations:
// unknown annotation types, records without a url, and records without a
// coherent [start,end) range are skipped because they can never locate text.
func parseChatURLCitations(raw json.RawMessage) []chatGeminiCitation {
	if len(raw) == 0 {
		return nil
	}
	var annotations []map[string]any
	if err := kitutil.Unmarshal(raw, &annotations); err != nil {
		return nil
	}
	citations := make([]chatGeminiCitation, 0, len(annotations))
	for _, annotation := range annotations {
		if strings.TrimSpace(kitutil.Interface2String(annotation["type"])) != "url_citation" {
			continue
		}
		citation, ok := annotation["url_citation"].(map[string]any)
		if !ok {
			citation = annotation
		}
		uri := strings.TrimSpace(kitutil.Interface2String(citation["url"]))
		if uri == "" {
			continue
		}
		start, startOK := annotationIndex(citation["start_index"])
		end, endOK := annotationIndex(citation["end_index"])
		if !startOK || !endOK || start < 0 || end <= start {
			continue
		}
		citations = append(citations, chatGeminiCitation{
			uri:   uri,
			title: strings.TrimSpace(kitutil.Interface2String(citation["title"])),
			start: start,
			end:   end,
		})
	}
	return citations
}

func geminiCitationDTO(c chatGeminiCitation) dto.GeminiCitation {
	start, end := c.start, c.end
	return dto.GeminiCitation{
		StartIndex: &start,
		EndIndex:   &end,
		URI:        c.uri,
		Title:      c.title,
	}
}

// chatAnnotationsToGeminiCitations converts url_citation annotations for a
// fully rendered answer, dropping ranges outside the text and keeping only
// the first record for each uri+range pair within the candidate.
func chatAnnotationsToGeminiCitations(raw json.RawMessage, text string) []dto.GeminiCitation {
	parsed := parseChatURLCitations(raw)
	if len(parsed) == 0 {
		return nil
	}
	textLen := utf8.RuneCountInString(text)
	seen := make(map[string]struct{})
	citations := make([]dto.GeminiCitation, 0, len(parsed))
	for _, citation := range parsed {
		if citation.end > textLen {
			continue
		}
		key := chatGeminiCitationKey(citation)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		citations = append(citations, geminiCitationDTO(citation))
	}
	return citations
}

// chatGeminiStreamCitations tracks one streamed OpenAI choice: its answer
// text length and the url_citation annotations that describe it. Upstream
// offsets are absolute, so a citation can arrive after the text or span
// fragments that have not been streamed yet.
type chatGeminiStreamCitations struct {
	runeLen int
	pending []chatGeminiCitation
	seen    map[string]struct{}
}

// appendText records a streamed answer fragment.
func (s *chatGeminiStreamCitations) appendText(text string) {
	s.runeLen += utf8.RuneCountInString(text)
}

// ingest parses one annotation delta. Citations locatable in the text
// accumulated so far are returned immediately; well-formed ranges beyond it
// are buffered when bufferFuture is true. Duplicate and permanently
// unlocatable records are dropped.
func (s *chatGeminiStreamCitations) ingest(raw json.RawMessage, bufferFuture bool) []dto.GeminiCitation {
	parsed := parseChatURLCitations(raw)
	if len(parsed) == 0 {
		return nil
	}
	var resolved []dto.GeminiCitation
	for _, citation := range parsed {
		key := chatGeminiCitationKey(citation)
		if _, ok := s.seen[key]; ok {
			continue
		}
		s.seen[key] = struct{}{}
		if citation.end <= s.runeLen {
			resolved = append(resolved, geminiCitationDTO(citation))
			continue
		}
		if bufferFuture {
			s.pending = append(s.pending, citation)
		}
	}
	return resolved
}

// flushBuffered locates buffered citations using text accumulated since the
// previous flush and keeps buffering the rest.
func (s *chatGeminiStreamCitations) flushBuffered() []dto.GeminiCitation {
	if len(s.pending) == 0 {
		return nil
	}
	resolved := make([]dto.GeminiCitation, 0)
	remaining := s.pending[:0]
	for _, citation := range s.pending {
		if citation.end <= s.runeLen {
			resolved = append(resolved, geminiCitationDTO(citation))
			continue
		}
		remaining = append(remaining, citation)
	}
	s.pending = remaining
	return resolved
}

// finish resolves every still-buffered citation against the now-complete
// answer and discards records whose range was never sent.
func (s *chatGeminiStreamCitations) finish() []dto.GeminiCitation {
	resolved := s.flushBuffered()
	s.pending = nil
	return resolved
}
