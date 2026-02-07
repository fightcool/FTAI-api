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
	// ToAPIs 返回的视频结果
	Result *struct {
		Type string `json:"type"`
		Data []struct {
			URL    string `json:"url"`
			Format string `json:"format"`
		} `json:"data"`
	} `json:"result,omitempty"`
	Error *struct {
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
	// ToAPIs 使用 /v1/videos/generations 路径
	defaultPaths := map[string]string{
		constant.TaskActionGenerate:     "/v1/videos/generations",
		constant.TaskActionTextGenerate: "/v1/videos/generations",
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

	// 兜底：默认使用 /v1/videos/generations
	return "/v1/videos/generations"
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

	// 默认查询端点：/v1/videos/generations/{task_id}（ToAPIs 格式）
	uri := fmt.Sprintf("%s/v1/videos/generations/%s", baseUrl, taskID)

	// 如果有自定义的查询端点路径，使用自定义路径
	if fetchPath, ok := body["fetch_path"].(string); ok && fetchPath != "" {
		uri = fmt.Sprintf("%s%s/%s", baseUrl, fetchPath, taskID)
	}

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
		// ToAPIs 返回的视频 URL 在 result.data[0].url
		if resTask.Result != nil && len(resTask.Result.Data) > 0 && resTask.Result.Data[0].URL != "" {
			taskResult.Url = resTask.Result.Data[0].URL
		} else {
			// 兜底：使用 FT-API 的 /content 端点
			taskResult.Url = fmt.Sprintf("%s/v1/videos/%s/content", system_setting.ServerAddress, resTask.ID)
		}
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
	// 从数据库任务记录构造包含最新状态和视频 URL 的响应
	status := "processing"
	switch task.Status {
	case model.TaskStatusSuccess:
		status = "completed"
	case model.TaskStatusFailure:
		status = "failed"
	case model.TaskStatusQueued:
		status = "queued"
	case model.TaskStatusInProgress:
		status = "processing"
	}

	resp := map[string]any{
		"id":       task.TaskID,
		"object":   "generation.task",
		"status":   status,
		"progress": 100,
	}

	// FailReason 在任务成功时存储的是视频 URL
	if task.Status == model.TaskStatusSuccess && task.FailReason != "" {
		resp["result"] = map[string]any{
			"type": "video",
			"data": []map[string]any{
				{"url": task.FailReason, "format": "mp4"},
			},
		}
		resp["url"] = task.FailReason
	} else if task.Status == model.TaskStatusFailure {
		resp["error"] = map[string]any{
			"message": task.FailReason,
		}
	}

	return common.Marshal(resp)
}
