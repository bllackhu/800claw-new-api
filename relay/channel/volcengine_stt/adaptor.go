package volcengine_stt

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	// volc.bigasr.auc_turbo 极速版识别的资源 ID
	volcSTTResourceID = "volc.bigasr.auc_turbo"
	// 成功码在响应头 X-Api-Status-Code
	volcSTTSuccessCode = "20000000"
	// 上传音频上限（火山文档建议 20M 以内）
	maxAudioFileSize = 20 * 1024 * 1024
)

type Adaptor struct {
	requestId      string
	responseFormat string
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info.RelayMode != relayconstant.RelayModeAudioTranscription {
		return "", errors.New("volcengine_stt: unsupported relay mode")
	}
	baseUrl := info.ChannelBaseUrl
	if baseUrl == "" {
		baseUrl = constant.ChannelBaseURLs[constant.ChannelTypeVolcengineSTT]
	}
	return fmt.Sprintf("%s/api/v3/auc/bigmodel/recognize/flash", baseUrl), nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	if info.RelayMode != relayconstant.RelayModeAudioTranscription {
		return errors.New("volcengine_stt: unsupported relay mode")
	}
	req.Set("Content-Type", "application/json")
	// 新控制台：单个 X-Api-Key；旧控制台：appid|access_token
	if strings.Contains(info.ApiKey, "|") {
		appID, accessKey, err := parseVolcengineAuth(info.ApiKey)
		if err != nil {
			return err
		}
		req.Set("X-Api-App-Key", appID)
		req.Set("X-Api-Access-Key", accessKey)
	} else {
		req.Set("X-Api-Key", info.ApiKey)
	}
	req.Set("X-Api-Resource-Id", volcSTTResourceID)
	if a.requestId == "" {
		a.requestId = uuid.New().String()
	}
	req.Set("X-Api-Request-Id", a.requestId)
	req.Set("X-Api-Sequence", "-1")
	return nil
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	if info.RelayMode != relayconstant.RelayModeAudioTranscription {
		return nil, errors.New("volcengine_stt: unsupported audio relay mode")
	}
	a.responseFormat = request.ResponseFormat

	formData, err := common.ParseMultipartFormReusable(c)
	if err != nil {
		return nil, fmt.Errorf("failed to parse multipart form: %w", err)
	}
	fileHeaders := formData.File["file"]
	if len(fileHeaders) == 0 {
		return nil, errors.New("file is required")
	}
	fileHeader := fileHeaders[0]
	if fileHeader.Size > maxAudioFileSize {
		return nil, fmt.Errorf("audio file too large: %d bytes (max %d)", fileHeader.Size, maxAudioFileSize)
	}
	file, err := fileHeader.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open audio file: %w", err)
	}
	defer file.Close()

	audioData, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read audio file: %w", err)
	}

	// 本地测量时长作为计费兜底（上游响应有 duration 时在 DoResponse 覆盖）
	if duration, err := common.GetAudioDuration(c.Request.Context(), bytes.NewReader(audioData), filepath.Ext(fileHeader.Filename)); err == nil {
		c.Set(string(constant.ContextKeySTTDurationMs), int(duration*1000))
	}

	audioBase64 := base64.StdEncoding.EncodeToString(audioData)

	appKey := info.ApiKey
	if strings.Contains(appKey, "|") {
		appKey = strings.Split(appKey, "|")[0]
	}

	body := map[string]any{
		"user":    map[string]any{"uid": appKey},
		"audio":   map[string]any{"data": audioBase64},
		"request": map[string]any{"model_name": "bigmodel"},
	}
	jsonData, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal volcengine STT request: %w", err)
	}
	a.requestId = uuid.New().String()
	return bytes.NewReader(jsonData), nil
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	if info.RelayMode != relayconstant.RelayModeAudioTranscription {
		return nil, types.NewErrorWithStatusCode(
			errors.New("volcengine_stt: unsupported relay mode"),
			types.ErrorCodeBadResponse, http.StatusBadRequest)
	}

	// 火山引擎的成功状态在响应头 X-Api-Status-Code，而不是 HTTP 状态码
	if statusCode := resp.Header.Get("X-Api-Status-Code"); statusCode != volcSTTSuccessCode {
		msg := resp.Header.Get("X-Api-Message")
		if msg == "" {
			msg = fmt.Sprintf("volcengine STT failed, X-Api-Status-Code: %s", statusCode)
		}
		return nil, types.NewErrorWithStatusCode(errors.New(msg), types.ErrorCodeBadResponse, http.StatusBadRequest)
	}

	bodyBytes, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, types.NewErrorWithStatusCode(readErr, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}

	var volcResp VolcengineRecognizeResponse
	if unmarshalErr := json.Unmarshal(bodyBytes, &volcResp); unmarshalErr != nil {
		return nil, types.NewErrorWithStatusCode(unmarshalErr, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	text := volcResp.Result.Text
	durationMs := volcResp.AudioInfo.Duration
	if durationMs > 0 {
		c.Set(string(constant.ContextKeySTTDurationMs), int(durationMs))
	}

	outBody, buildErr := buildOpenAIResponse(a.responseFormat, text, volcResp)
	if buildErr != nil {
		return nil, types.NewErrorWithStatusCode(buildErr, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	service.IOCopyBytesGracefully(c, resp, outBody)

	usageObj := &dto.Usage{
		PromptTokens:     info.GetEstimatePromptTokens(),
		CompletionTokens: 0,
		TotalTokens:      info.GetEstimatePromptTokens(),
	}
	if durationMs > 0 {
		usageObj.PromptTokensDetails.AudioTokens = int(durationMs / 1000)
		usageObj.TotalTokens = usageObj.PromptTokens + usageObj.CompletionTokens
	}
	return usageObj, nil
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	return nil, errors.New("volcengine_stt: unsupported")
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, errors.New("volcengine_stt: unsupported")
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return nil, errors.New("volcengine_stt: unsupported")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return nil, errors.New("volcengine_stt: unsupported")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return nil, errors.New("volcengine_stt: unsupported")
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	return nil, errors.New("volcengine_stt: unsupported")
}

func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	return nil, errors.New("volcengine_stt: unsupported")
}
