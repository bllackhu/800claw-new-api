package volcengine_stt

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

var ModelList = []string{
	"whisper-1",
	"volcengine-bigmodel",
}

var ChannelName = "volcengine_stt"

// VolcengineRecognizeResponse 极速版识别响应体（https://docs.volcengine.com/docs/6561/1631584）
type VolcengineRecognizeResponse struct {
	AudioInfo struct {
		Duration int64 `json:"duration"` // 毫秒
	} `json:"audio_info"`
	Result struct {
		Text       string                `json:"text"`
		Utterances []VolcengineUtterance `json:"utterances"`
	} `json:"result"`
}

type VolcengineUtterance struct {
	StartTime int64  `json:"start_time"` // 毫秒
	EndTime   int64  `json:"end_time"`   // 毫秒
	Text      string `json:"text"`
}

func parseVolcengineAuth(apiKey string) (appID, accessKey string, err error) {
	parts := strings.Split(apiKey, "|")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid api key format, expected: appid|access_token")
	}
	return parts[0], parts[1], nil
}

// buildOpenAIResponse 将火山识别结果按下游 response_format 转回 OpenAI 兼容格式
func buildOpenAIResponse(responseFormat, text string, resp VolcengineRecognizeResponse) ([]byte, error) {
	switch responseFormat {
	case "text":
		return []byte(text), nil
	case "verbose_json":
		out := dto.WhisperVerboseJSONResponse{
			Task:     "transcribe",
			Duration: float64(resp.AudioInfo.Duration) / 1000,
			Text:     text,
			Segments: buildSegments(resp),
		}
		return common.Marshal(out)
	case "srt", "vtt":
		return []byte(buildSubtitle(responseFormat, resp.Result.Utterances)), nil
	default: // "json"
		return common.Marshal(dto.AudioResponse{Text: text})
	}
}

func buildSegments(resp VolcengineRecognizeResponse) []dto.Segment {
	segments := make([]dto.Segment, 0, len(resp.Result.Utterances))
	for i, u := range resp.Result.Utterances {
		segments = append(segments, dto.Segment{
			Id:    i,
			Start: float64(u.StartTime) / 1000,
			End:   float64(u.EndTime) / 1000,
			Text:  u.Text,
		})
	}
	return segments
}

func buildSubtitle(format string, utterances []VolcengineUtterance) string {
	if format != "srt" && format != "vtt" {
		return ""
	}
	var sb strings.Builder
	if format == "vtt" {
		sb.WriteString("WEBVTT\n\n")
	}
	for i, u := range utterances {
		if format == "srt" {
			sb.WriteString(strconv.Itoa(i + 1))
			sb.WriteString("\n")
			sb.WriteString(formatSRTTime(u.StartTime))
			sb.WriteString(" --> ")
			sb.WriteString(formatSRTTime(u.EndTime))
			sb.WriteString("\n")
			sb.WriteString(u.Text)
			sb.WriteString("\n\n")
		} else {
			sb.WriteString(formatVTTTime(u.StartTime))
			sb.WriteString(" --> ")
			sb.WriteString(formatVTTTime(u.EndTime))
			sb.WriteString("\n")
			sb.WriteString(u.Text)
			sb.WriteString("\n\n")
		}
	}
	return sb.String()
}

func formatSRTTime(ms int64) string {
	totalSec := ms / 1000
	remainMs := ms % 1000
	h := totalSec / 3600
	m := (totalSec % 3600) / 60
	s := totalSec % 60
	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, remainMs)
}

func formatVTTTime(ms int64) string {
	totalSec := ms / 1000
	remainMs := ms % 1000
	h := totalSec / 3600
	m := (totalSec % 3600) / 60
	s := totalSec % 60
	return fmt.Sprintf("%02d:%02d:%02d.%03d", h, m, s, remainMs)
}
