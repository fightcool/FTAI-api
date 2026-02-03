package sora

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

// ============================
// Request / Response structures
// ============================

type ContentItem struct {
	Type     string    `json:"type"`                // "text" or "image_url"
	Text     string    `json:"text,omitempty"`      // for text type
	ImageURL *ImageURL `json:"image_url,omitempty"` // for image_url type
}

type ImageURL struct {
	URL string `json:"url"`
}

type responseTask struct {
	ID                 string `json:"id"`
	TaskID             string `json:"task_id,omitempty"` //兼容旧接口
	Object             string `json:"object"`
	Model              string `json:"model"`
	Status             string `json:"status"`
	Progress           int    `json:"progress"`
	CreatedAt          int64  `json:"created_at"`
	CompletedAt        int64  `json:"completed_at,omitempty"`
	ExpiresAt          int64  `json:"expires_at,omitempty"`
	Seconds            string `json:"seconds,omitempty"`
	Size               string `json:"size,omitempty"`
	RemixedFromVideoID string `json:"remixed_from_video_id,omitempty"`
	Error              *struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

// ============================
// Adaptor implementation
// ============================

type TaskAdaptor struct {
	ChannelType int
	apiKey      string
	baseURL     string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

func validateRemixRequest(c *gin.Context) *dto.TaskError {
	var req struct {
		Prompt string `json:"prompt"`
	}
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("field prompt is required"), "invalid_request", http.StatusBadRequest)
	}
	return nil
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	if info.Action == constant.TaskActionRemix {
		return validateRemixRequest(c)
	}
	return relaycommon.ValidateMultipartDirect(c, info)
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info.Action == constant.TaskActionRemix {
		return fmt.Sprintf("%s/v1/videos/%s/remix", a.baseURL, info.OriginTaskID), nil
	}
	// 获取端点路径，支持从渠道配置中自定义
	path := a.getEndpointPath(info)
	return fmt.Sprintf("%s%s", a.baseURL, path), nil
}

// getEndpointPath 获取端点路径，支持从渠道配置中自定义
func (a *TaskAdaptor) getEndpointPath(info *relaycommon.RelayInfo) string {
	// 默认端点路径映射
	defaultPaths := map[string]string{
		constant.TaskActionGenerate:     "/v1/videos",
		constant.TaskActionTextGenerate: "/v1/videos",
	}

	// 如果渠道配置了自定义端点路径，优先使用
	if info.ChannelMeta != nil {
		if info.ChannelMeta.ChannelSetting.EndpointPaths != nil {
			if customPath, ok := info.ChannelMeta.ChannelSetting.EndpointPaths[info.Action]; ok && customPath != "" {
				common.SysLog(fmt.Sprintf("Sora adaptor: Using custom endpoint path: %s for action: %s", customPath, info.Action))
				return customPath
			}
		}
	}

	// 使用默认路径
	if path, ok := defaultPaths[info.Action]; ok {
		return path
	}

	// 兜底：默认使用 /v1/videos
	return "/v1/videos"
}

// BuildRequestHeader sets required headers.
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", c.Request.Header.Get("Content-Type"))
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	cachedBody, err := common.GetRequestBody(c)
	if err != nil {
		return nil, errors.Wrap(err, "get_request_body_failed")
	}

	// 解析请求体以进行必要的转换
	var requestMap map[string]interface{}
	needsModification := false

	if err := common.Unmarshal(cachedBody, &requestMap); err == nil {
		// 1. 如果模型被映射了，更新model字段
		if info.IsModelMapped {
			requestMap["model"] = info.UpstreamModelName
			needsModification = true
		}

		// 2. 转换size参数（large/medium/small → 具体分辨率）
		if size, ok := requestMap["size"].(string); ok {
			var newSize string
			switch size {
			case "large":
				// 横屏用1280x720，竖屏用720x1280
				if orientation, _ := requestMap["orientation"].(string); orientation == "portrait" {
					newSize = "720x1280"
				} else {
					newSize = "1280x720" // 默认横屏
				}
			case "medium", "small":
				// 中小尺寸也用标清
				if orientation, _ := requestMap["orientation"].(string); orientation == "portrait" {
					newSize = "720x1280"
				} else {
					newSize = "1280x720"
				}
			}
			if newSize != "" {
				requestMap["size"] = newSize
				needsModification = true
			}
		}

		// 3. 如果有修改，重新序列化
		if needsModification {
			if modifiedBody, err := common.Marshal(requestMap); err == nil {
				return bytes.NewReader(modifiedBody), nil
			}
		}
	}

	return bytes.NewReader(cachedBody), nil
}

// DoRequest delegates to common helper.
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse handles upstream response, returns taskID etc.
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, _ *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	// Parse Sora response
	var dResp responseTask
	if err := common.Unmarshal(responseBody, &dResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	if dResp.ID == "" {
		if dResp.TaskID == "" {
			taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
			return
		}
		dResp.ID = dResp.TaskID
		dResp.TaskID = ""
	}

	c.JSON(http.StatusOK, dResp)
	return dResp.ID, responseBody, nil
}

// FetchTask fetch task status
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	uri := fmt.Sprintf("%s/v1/videos/%s", baseUrl, taskID)

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	resTask := responseTask{}
	if err := common.Unmarshal(respBody, &resTask); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	taskResult := relaycommon.TaskInfo{
		Code: 0,
	}

	switch resTask.Status {
	case "queued", "pending":
		taskResult.Status = model.TaskStatusQueued
	case "processing", "in_progress":
		taskResult.Status = model.TaskStatusInProgress
	case "completed":
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Url = fmt.Sprintf("%s/v1/videos/%s/content", system_setting.ServerAddress, resTask.ID)
	case "failed", "cancelled":
		taskResult.Status = model.TaskStatusFailure
		if resTask.Error != nil {
			taskResult.Reason = resTask.Error.Message
		} else {
			taskResult.Reason = "task failed"
		}
	default:
	}
	if resTask.Progress > 0 && resTask.Progress < 100 {
		taskResult.Progress = fmt.Sprintf("%d%%", resTask.Progress)
	}

	return &taskResult, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	return task.Data, nil
}
