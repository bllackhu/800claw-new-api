package volcengine_stt

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleResponse() VolcengineRecognizeResponse {
	var resp VolcengineRecognizeResponse
	err := json.Unmarshal([]byte(`{
		"audio_info": {"duration": 2499},
		"result": {
			"text": "关闭透传。",
			"utterances": [{
				"start_time": 450, "end_time": 1530, "text": "关闭透传。"
			}]
		}
	}`), &resp)
	if err != nil {
		panic(err)
	}
	return resp
}

func TestBuildOpenAIResponseJSON(t *testing.T) {
	out, err := buildOpenAIResponse("json", "关闭透传。", sampleResponse())
	require.NoError(t, err)
	var parsed map[string]string
	require.NoError(t, json.Unmarshal(out, &parsed))
	assert.Equal(t, "关闭透传。", parsed["text"])
}

func TestBuildOpenAIResponseText(t *testing.T) {
	out, err := buildOpenAIResponse("text", "关闭透传。", sampleResponse())
	require.NoError(t, err)
	assert.Equal(t, "关闭透传。", string(out))
}

func TestBuildOpenAIResponseVerboseJSON(t *testing.T) {
	out, err := buildOpenAIResponse("verbose_json", "关闭透传。", sampleResponse())
	require.NoError(t, err)
	var parsed struct {
		Text     string `json:"text"`
		Duration float64 `json:"duration"`
		Segments []struct {
			Start float64 `json:"start"`
			End   float64 `json:"end"`
			Text  string  `json:"text"`
		} `json:"segments"`
	}
	require.NoError(t, json.Unmarshal(out, &parsed))
	assert.Equal(t, "关闭透传。", parsed.Text)
	assert.Equal(t, 2.499, parsed.Duration)
	require.Len(t, parsed.Segments, 1)
	assert.Equal(t, 0.45, parsed.Segments[0].Start)
	assert.Equal(t, 1.53, parsed.Segments[0].End)
}

func TestBuildOpenAIResponseSRT(t *testing.T) {
	out, err := buildOpenAIResponse("srt", "关闭透传。", sampleResponse())
	require.NoError(t, err)
	assert.Contains(t, string(out), "00:00:00,450 --> 00:00:01,530")
	assert.Contains(t, string(out), "关闭透传。")
}

func TestBuildOpenAIResponseVTT(t *testing.T) {
	out, err := buildOpenAIResponse("vtt", "关闭透传。", sampleResponse())
	require.NoError(t, err)
	assert.Contains(t, string(out), "WEBVTT")
	assert.Contains(t, string(out), "00:00:00.450 --> 00:00:01.530")
}

func TestParseVolcengineAuth(t *testing.T) {
	appID, accessKey, err := parseVolcengineAuth("12345|secret")
	require.NoError(t, err)
	assert.Equal(t, "12345", appID)
	assert.Equal(t, "secret", accessKey)

	_, _, err = parseVolcengineAuth("singlekey")
	assert.Error(t, err)
}

func TestModelListIncludesWhisper(t *testing.T) {
	assert.Contains(t, ModelList, "whisper-1")
}
