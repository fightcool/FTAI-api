package unified_video

import (
	"bytes"
	"encoding/json"
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

// upstreamVideoRequest 上游 API 期望的请求格式
type upstreamVideoRequest struct {
	Model          string   `json:"model"`
	Prompt         string   `json:"prompt"`
	Images         []string `json:"images,omitempty"`
	EnhancePrompt  bool     `json:"enhance_prompt,omitempty"`
	EnableUpsample bool     `json:"enable_upsample,omitempty"`
	AspectRatio    string   `json:"aspect_ratio,omitempty"`
}

// responseTask 上游 API 响应格式
// 🔥 支持两种格式：
// 1. 扁平格式：{ "id": "xxx", "status": "completed", ... }
// 2. 嵌套格式：{ "status": "FAILURE", "data": { "id": "xxx", "status": "failed", ... } }
type responseTask struct {
	// 顶层字段（扁平格式或嵌套格式的外层）
	// 🔥 ID 使用 interface{} 兼容数字和任意字符串两种格式
	// 数字格式: 123 或 "123"
	// 字符串格式: "veo3.1-fast:1769574608-xxx"
	ID               interface{} `json:"id"`
	TaskID           string      `json:"task_id,omitempty"`
	Object           string      `json:"object,omitempty"`
	Status           string      `json:"status"`
	StatusUpdateTime int64       `json:"status_update_time,omitempty"`
	Progress         int         `json:"progress,omitempty"`
	CreatedAt        int64       `json:"created_at,omitempty"`
	// 🔥 失败原因字段（上游返回 fail_reason）
	FailReason string `json:"fail_reason,omitempty"`
	// 视频 URL 字段（不同上游 API 可能使用不同字段名）
	URL       string `json:"url,omitempty"`
	VideoURL  string `json:"video_url,omitempty"`
	OutputURL string `json:"output_url,omitempty"`
	// 🔥 Error 使用 json.RawMessage 兼容字符串和结构体两种格式
	// 上游可能返回 "error": "message" 或 "error": {"message": "...", "code": "..."}
	Error json.RawMessage `json:"error,omitempty"`
	// 🔥 嵌套的 data 字段（某些上游 API 使用嵌套结构）
	Data *responseTaskData `json:"data,omitempty"`
}

// responseTaskData 嵌套的 data 字段结构
type responseTaskData struct {
	ID               string `json:"id"`
	Status           string `json:"status"`
	StatusUpdateTime int64  `json:"status_update_time,omitempty"`
	VideoURL         string `json:"video_url,omitempty"`
	Error            string `json:"error,omitempty"`
	ErrorMessage     string `json:"error_message,omitempty"`
	// 🔥 detail 字段包含更详细的错误信息
	Detail *responseTaskDetail `json:"detail,omitempty"`
}

// responseTaskDetail 详细信息结构
type responseTaskDetail struct {
	Message     string `json:"message,omitempty"`
	Error       string `json:"error,omitempty"`
	Code        string `json:"code,omitempty"`
	Description string `json:"description,omitempty"`
}

// ============================
// Adaptor implementation
// ============================

type TaskAdaptor struct {
	ChannelType   int
	apiKey        string
	baseURL       string
	convertedBody []byte
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate)
}

// BuildRequestURL 构建请求 URL，支持自定义端点
func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	path := a.getEndpointPath(info)
	url := fmt.Sprintf("%s%s", a.baseURL, path)
	common.SysLog(fmt.Sprintf("UnifiedVideo adaptor: Request URL: %s", url))
	return url, nil
}

// getEndpointPath 获取端点路径，支持从渠道配置中自定义
func (a *TaskAdaptor) getEndpointPath(info *relaycommon.RelayInfo) string {
	// 默认端点路径
	defaultPath := "/v1/video/create"

	// 如果渠道配置了自定义端点路径，优先使用
	if info.ChannelMeta != nil && info.ChannelMeta.ChannelSetting.EndpointPaths != nil {
		if customPath, ok := info.ChannelMeta.ChannelSetting.EndpointPaths[info.Action]; ok && customPath != "" {
			common.SysLog(fmt.Sprintf("UnifiedVideo adaptor: Using custom endpoint path: %s", customPath))
			return customPath
		}
		// 也支持通用的 "generate" key
		if customPath, ok := info.ChannelMeta.ChannelSetting.EndpointPaths["generate"]; ok && customPath != "" {
			common.SysLog(fmt.Sprintf("UnifiedVideo adaptor: Using custom endpoint path: %s", customPath))
			return customPath
		}
	}

	return defaultPath
}

// BuildRequestHeader 设置请求头
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return nil
}

// BuildRequestBody 构建请求体，将 ftai-movies 格式转换为上游 API 格式
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	v, exists := c.Get("task_request")
	if !exists {
		return nil, fmt.Errorf("request not found in context")
	}
	req := v.(relaycommon.TaskSubmitReq)

	// 转换为上游 API 格式
	upstreamReq := a.convertToUpstreamFormat(&req, info)

	data, err := json.Marshal(upstreamReq)
	if err != nil {
		return nil, errors.Wrap(err, "marshal upstream request failed")
	}

	common.SysLog(fmt.Sprintf("UnifiedVideo adaptor: Converted request: %s", string(data)))
	a.convertedBody = data
	return bytes.NewReader(data), nil
}

// convertToUpstreamFormat 将 ftai-movies 的请求格式转换为上游 API 格式
func (a *TaskAdaptor) convertToUpstreamFormat(req *relaycommon.TaskSubmitReq, info *relaycommon.RelayInfo) *upstreamVideoRequest {
	upstreamReq := &upstreamVideoRequest{
		Prompt:        req.Prompt,
		EnhancePrompt: true, // 默认开启中文转英文
	}

	// 使用映射后的模型名称
	if info.UpstreamModelName != "" {
		upstreamReq.Model = info.UpstreamModelName
	} else {
		upstreamReq.Model = req.Model
	}

	// 处理图片：将 image 和 lastFrame 合并到 images 数组
	var images []string
	if req.Image != "" {
		images = append(images, req.Image)
	}
	// 也支持 Images 数组
	if len(req.Images) > 0 {
		images = append(images, req.Images...)
	}

	// 从 metadata 中提取 lastFrame
	if req.Metadata != nil {
		if lastFrame, ok := req.Metadata["lastFrame"].(string); ok && lastFrame != "" {
			images = append(images, lastFrame)
		}
		// 处理 aspectRatio
		if aspectRatio, ok := req.Metadata["aspectRatio"].(string); ok && aspectRatio != "" {
			upstreamReq.AspectRatio = aspectRatio
		}
		// 处理 enhance_prompt
		if enhancePrompt, ok := req.Metadata["enhance_prompt"].(bool); ok {
			upstreamReq.EnhancePrompt = enhancePrompt
		}
		// 处理 enable_upsample
		if enableUpsample, ok := req.Metadata["enable_upsample"].(bool); ok {
			upstreamReq.EnableUpsample = enableUpsample
		}
	}

	if len(images) > 0 {
		upstreamReq.Images = images
	}

	// 如果没有设置 aspectRatio，根据 size 推断
	if upstreamReq.AspectRatio == "" && req.Size != "" {
		upstreamReq.AspectRatio = sizeToAspectRatio(req.Size)
	}

	return upstreamReq
}

// sizeToAspectRatio 将尺寸转换为宽高比
func sizeToAspectRatio(size string) string {
	switch size {
	case "1920x1080", "1280x720":
		return "16:9"
	case "1080x1920", "720x1280":
		return "9:16"
	case "1024x1024":
		return "1:1"
	default:
		if strings.Contains(size, "x") {
			parts := strings.Split(size, "x")
			if len(parts) == 2 {
				if parts[0] > parts[1] {
					return "16:9"
				} else if parts[0] < parts[1] {
					return "9:16"
				}
				return "1:1"
			}
		}
		return "16:9"
	}
}

// DoRequest 发送请求
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse 处理响应
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	common.SysLog(fmt.Sprintf("UnifiedVideo adaptor: Response status: %d, body: %s", resp.StatusCode, string(responseBody)))

	// 检查 HTTP 状态码
	if resp.StatusCode != http.StatusOK {
		taskErr = service.TaskErrorWrapper(
			fmt.Errorf("upstream returned status %d: %s", resp.StatusCode, string(responseBody)),
			"upstream_error",
			resp.StatusCode,
		)
		return
	}

	// 解析响应
	var dResp responseTask
	if err := common.Unmarshal(responseBody, &dResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	// 🔥 获取任务 ID（兼容 数字、json.Number、string 等多种格式）
	taskIDStr := ""
	switch v := dResp.ID.(type) {
	case string:
		taskIDStr = v
	case float64:
		taskIDStr = fmt.Sprintf("%.0f", v)
	case json.Number:
		taskIDStr = v.String()
	case nil:
		// ID 为空，尝试使用 TaskID
	default:
		taskIDStr = fmt.Sprintf("%v", v)
	}
	if taskIDStr == "" && dResp.TaskID != "" {
		taskIDStr = dResp.TaskID
	}
	if taskIDStr == "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty, response: %s", string(responseBody)), "invalid_response", http.StatusInternalServerError)
		return
	}

	// 返回 OpenAI 兼容格式
	openAIResp := dto.NewOpenAIVideo()
	openAIResp.ID = taskIDStr
	openAIResp.TaskID = taskIDStr
	openAIResp.Status = dResp.Status
	openAIResp.Model = info.OriginModelName
	if dResp.CreatedAt > 0 {
		openAIResp.CreatedAt = dResp.CreatedAt
	}

	c.JSON(http.StatusOK, openAIResp)
	return taskIDStr, responseBody, nil
}

// FetchTask 获取任务状态
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	// 默认使用 /v1/video/query?id=xxx 端点（VectorEngine 格式）
	uri := fmt.Sprintf("%s/v1/video/query?id=%s", baseUrl, taskID)

	// 如果有自定义的查询端点，使用自定义端点
	if fetchPath, ok := body["fetch_path"].(string); ok && fetchPath != "" {
		uri = fmt.Sprintf("%s%s?id=%s", baseUrl, fetchPath, taskID)
	}

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")

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

	// 🔥 优先使用嵌套 data 字段中的信息（某些上游 API 使用嵌套结构）
	// 上游返回格式: { "status": "FAILURE", "data": { "id": "xxx", "status": "failed", "video_url": "..." } }
	effectiveStatus := resTask.Status
	// 🔥 ID 兼容 数字、json.Number、string 等多种格式
	effectiveID := ""
	switch v := resTask.ID.(type) {
	case string:
		effectiveID = v
	case float64:
		effectiveID = fmt.Sprintf("%.0f", v)
	case json.Number:
		effectiveID = v.String()
	case nil:
		// ID 为空
	default:
		effectiveID = fmt.Sprintf("%v", v)
	}
	effectiveVideoURL := resTask.VideoURL
	effectiveFailReason := resTask.FailReason

	if resTask.Data != nil {
		// 如果有嵌套的 data 字段，优先使用 data 内部的状态
		if resTask.Data.Status != "" {
			effectiveStatus = resTask.Data.Status
		}
		if resTask.Data.ID != "" {
			effectiveID = resTask.Data.ID
		}
		if resTask.Data.VideoURL != "" {
			effectiveVideoURL = resTask.Data.VideoURL
		}
		// 如果 data 内部有错误信息，使用它作为失败原因
		if resTask.Data.Error != "" {
			effectiveFailReason = resTask.Data.Error
		}
		if resTask.Data.ErrorMessage != "" && effectiveFailReason == "" {
			effectiveFailReason = resTask.Data.ErrorMessage
		}
		// 🔥 如果 detail 字段有更详细的错误信息，追加到失败原因
		if resTask.Data.Detail != nil {
			if resTask.Data.Detail.Message != "" && effectiveFailReason == "" {
				effectiveFailReason = resTask.Data.Detail.Message
			}
			if resTask.Data.Detail.Description != "" && effectiveFailReason == "" {
				effectiveFailReason = resTask.Data.Detail.Description
			}
		}
	}

	// 🔥 从上游响应中提取视频 URL（不同 API 可能使用不同字段名）
	upstreamVideoURL := effectiveVideoURL
	if upstreamVideoURL == "" {
		upstreamVideoURL = resTask.URL
	}
	if upstreamVideoURL == "" {
		upstreamVideoURL = resTask.OutputURL
	}

	// 🔥 统一转换为小写进行状态匹配，解决上游返回大写状态（如 "FAILURE"）的问题
	statusLower := strings.ToLower(effectiveStatus)
	switch statusLower {
	case "queued", "pending", "not_start":
		// 🔥 添加 not_start 状态（上游可能返回 "NOT_START"）
		taskResult.Status = model.TaskStatusQueued
	case "processing", "in_progress", "video_generating", "image_uploading", "image_processing", "running":
		// 🔥 添加 video_generating、running 等中间状态的处理
		taskResult.Status = model.TaskStatusInProgress
	case "completed", "succeeded", "success":
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Url = fmt.Sprintf("%s/v1/videos/%s/content", system_setting.ServerAddress, effectiveID)
		taskResult.Progress = "100%" // 任务完成时强制设置进度为100%
		// 🔥 设置 RemoteUrl 为上游实际视频 URL，供 VideoProxy 使用
		if upstreamVideoURL != "" {
			taskResult.RemoteUrl = upstreamVideoURL
		}
	case "failed", "cancelled", "error", "failure":
		// 🔥 添加 "failure" 状态处理（上游可能返回 "FAILURE"）
		taskResult.Status = model.TaskStatusFailure
		// 🔥 解析 Error 字段（可能是字符串或结构体）
		if len(resTask.Error) > 0 {
			// 尝试解析为字符串
			var errStr string
			if err := json.Unmarshal(resTask.Error, &errStr); err == nil {
				taskResult.Reason = errStr
			} else {
				// 尝试解析为结构体
				var errObj struct {
					Message string `json:"message"`
					Code    string `json:"code"`
				}
				if err := json.Unmarshal(resTask.Error, &errObj); err == nil && errObj.Message != "" {
					taskResult.Reason = errObj.Message
				}
			}
		}
		// 如果 Error 字段没有解析出原因，使用其他来源
		if taskResult.Reason == "" && effectiveFailReason != "" {
			taskResult.Reason = effectiveFailReason
		}
		if taskResult.Reason == "" {
			taskResult.Reason = "task failed"
		}
	default:
		// 未知状态，保持 pending
		taskResult.Status = model.TaskStatusQueued
	}

	if resTask.Progress > 0 && resTask.Progress < 100 {
		taskResult.Progress = fmt.Sprintf("%d%%", resTask.Progress)
	}

	return &taskResult, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = task.TaskID
	openAIVideo.Status = task.Status.ToVideoStatus()
	openAIVideo.SetProgressStr(task.Progress)

	// 🔥 任务成功时，FailReason 字段存储的是视频 URL（历史遗留设计）
	if task.Status == model.TaskStatusSuccess && task.FailReason != "" {
		openAIVideo.SetMetadata("url", task.FailReason)
	}

	if task.Data != nil {
		var resTask responseTask
		if err := common.Unmarshal(task.Data, &resTask); err == nil {
			if resTask.CreatedAt > 0 {
				openAIVideo.CreatedAt = resTask.CreatedAt
			}
		}
	}

	jsonData, _ := common.Marshal(openAIVideo)
	return jsonData, nil
}
