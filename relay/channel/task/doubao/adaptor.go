package doubao

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

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
	Role     string    `json:"role,omitempty"`      // 图片角色: first_frame, last_frame, reference_image
}

type ImageURL struct {
	URL string `json:"url"`
}

type requestPayload struct {
	Model         string        `json:"model"`
	Content       []ContentItem `json:"content"`
	Resolution    string        `json:"resolution,omitempty"`     // 分辨率: 480p, 720p, 1080p
	Ratio         string        `json:"ratio,omitempty"`          // 宽高比: 16:9, 4:3, 1:1, 3:4, 9:16, 21:9, adaptive
	Duration      int           `json:"duration,omitempty"`       // 时长(秒): 2-12
	Seed          int           `json:"seed,omitempty"`           // 随机种子
	CameraFixed   *bool         `json:"camera_fixed,omitempty"`   // 是否固定摄像头
	Watermark     *bool         `json:"watermark,omitempty"`      // 是否包含水印
	GenerateAudio *bool         `json:"generate_audio,omitempty"` // 是否生成音频 (Seedance 1.5 pro)
}

type responsePayload struct {
	ID string `json:"id"` // task_id
}

type responseTask struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Status  string `json:"status"`
	Content struct {
		VideoURL     string `json:"video_url"`
		LastFrameURL string `json:"last_frame_url,omitempty"` // 尾帧图像URL
	} `json:"content"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
	Seed            int    `json:"seed"`
	Resolution      string `json:"resolution"`
	Duration        int    `json:"duration"`
	Frames          int    `json:"frames,omitempty"`
	Ratio           string `json:"ratio"`
	FramesPerSecond int    `json:"framespersecond"`
	GenerateAudio   bool   `json:"generate_audio,omitempty"`
	Draft           bool   `json:"draft,omitempty"`
	DraftTaskID     string `json:"draft_task_id,omitempty"`
	ServiceTier     string `json:"service_tier,omitempty"`
	Usage           struct {
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	CreatedAt int64 `json:"created_at"`
	UpdatedAt int64 `json:"updated_at"`
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

// ValidateRequestAndSetAction parses body, validates fields and sets default action.
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	// Accept only POST /v1/video/generations as "generate" action.
	return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate)
}

// BuildRequestURL constructs the upstream URL.
func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/api/v3/contents/generations/tasks", a.baseURL), nil
}

// BuildRequestHeader sets required headers.
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

// BuildRequestBody converts request into Doubao specific format.
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	v, exists := c.Get("task_request")
	if !exists {
		return nil, fmt.Errorf("request not found in context")
	}
	req := v.(relaycommon.TaskSubmitReq)

	body, err := a.convertToRequestPayload(&req)
	if err != nil {
		return nil, errors.Wrap(err, "convert request payload failed")
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

// DoRequest delegates to common helper.
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse handles upstream response, returns taskID etc.
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	// Parse Doubao response
	var dResp responsePayload
	if err := json.Unmarshal(responseBody, &dResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	if dResp.ID == "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
		return
	}

	c.JSON(http.StatusOK, gin.H{"task_id": dResp.ID})
	return dResp.ID, responseBody, nil
}

// FetchTask fetch task status
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	uri := fmt.Sprintf("%s/api/v3/contents/generations/tasks/%s", baseUrl, taskID)

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
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

func (a *TaskAdaptor) convertToRequestPayload(req *relaycommon.TaskSubmitReq) (*requestPayload, error) {
	r := requestPayload{
		Model:   req.Model,
		Content: []ContentItem{},
	}

	// Add text prompt
	if req.Prompt != "" {
		r.Content = append(r.Content, ContentItem{
			Type: "text",
			Text: req.Prompt,
		})
	}

	// 处理图片 - 支持首帧、尾帧、参考图模式
	// 根据火山引擎官方 API 规范:
	// - 首帧图生视频: 1个image_url, role=first_frame (或不填)
	// - 首尾帧图生视频: 2个image_url, role分别为first_frame和last_frame
	// - 参考图生视频: 1-4个image_url, role=reference_image
	if req.HasImage() {
		// 检查是否有尾帧图片 (从 metadata 中获取)
		var lastFrameURL string
		if req.Metadata != nil {
			if lf, ok := req.Metadata["lastFrame"].(string); ok && lf != "" {
				lastFrameURL = lf
			}
		}

		// 检查是否是参考图模式
		isReferenceMode := false
		if req.Metadata != nil {
			if mode, ok := req.Metadata["mode"].(string); ok && mode == "reference-images" {
				isReferenceMode = true
			}
		}

		if isReferenceMode {
			// 参考图模式: 所有图片都是 reference_image
			for _, imgURL := range req.Images {
				r.Content = append(r.Content, ContentItem{
					Type:     "image_url",
					ImageURL: &ImageURL{URL: imgURL},
					Role:     "reference_image",
				})
			}
		} else if lastFrameURL != "" {
			// 首尾帧模式: 第一张是首帧，lastFrame是尾帧
			if len(req.Images) > 0 {
				r.Content = append(r.Content, ContentItem{
					Type:     "image_url",
					ImageURL: &ImageURL{URL: req.Images[0]},
					Role:     "first_frame",
				})
			}
			r.Content = append(r.Content, ContentItem{
				Type:     "image_url",
				ImageURL: &ImageURL{URL: lastFrameURL},
				Role:     "last_frame",
			})
		} else {
			// 首帧模式: 只有首帧图片
			for i, imgURL := range req.Images {
				role := ""
				if i == 0 {
					role = "first_frame" // 第一张图片作为首帧
				}
				r.Content = append(r.Content, ContentItem{
					Type:     "image_url",
					ImageURL: &ImageURL{URL: imgURL},
					Role:     role,
				})
			}
		}
	}

	// 从 metadata 中提取视频生成参数
	if req.Metadata != nil {
		// 分辨率: 480p, 720p, 1080p
		if resolution, ok := req.Metadata["resolution"].(string); ok && resolution != "" {
			r.Resolution = resolution
		}

		// 宽高比: 16:9, 4:3, 1:1, 3:4, 9:16, 21:9, adaptive
		if ratio, ok := req.Metadata["ratio"].(string); ok && ratio != "" {
			r.Ratio = ratio
		} else if aspectRatio, ok := req.Metadata["aspectRatio"].(string); ok && aspectRatio != "" {
			r.Ratio = aspectRatio
		}

		// 时长(秒)
		if duration, ok := req.Metadata["duration"].(float64); ok && duration > 0 {
			r.Duration = int(duration)
		}

		// 随机种子
		if seed, ok := req.Metadata["seed"].(float64); ok {
			r.Seed = int(seed)
		}

		// 是否固定摄像头
		if cameraFixed, ok := req.Metadata["camera_fixed"].(bool); ok {
			r.CameraFixed = &cameraFixed
		} else if cameraFixed, ok := req.Metadata["cameraFixed"].(bool); ok {
			r.CameraFixed = &cameraFixed
		}

		// 是否包含水印
		if watermark, ok := req.Metadata["watermark"].(bool); ok {
			r.Watermark = &watermark
		}

		// 是否生成音频 (Seedance 1.5 pro)
		if generateAudio, ok := req.Metadata["generate_audio"].(bool); ok {
			r.GenerateAudio = &generateAudio
		} else if generateAudio, ok := req.Metadata["generateAudio"].(bool); ok {
			r.GenerateAudio = &generateAudio
		}
	}

	// 从顶层参数中提取 (优先级高于 metadata)
	if req.Duration > 0 {
		r.Duration = req.Duration
	}

	return &r, nil
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	resTask := responseTask{}
	if err := json.Unmarshal(respBody, &resTask); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	taskResult := relaycommon.TaskInfo{
		Code: 0,
	}

	// Map Doubao status to internal status
	// 官方状态: queued, running, cancelled, succeeded, failed, expired
	switch resTask.Status {
	case "queued":
		taskResult.Status = model.TaskStatusQueued
		taskResult.Progress = "10%"
	case "running":
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "50%"
	case "succeeded":
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Progress = "100%"
		taskResult.Url = resTask.Content.VideoURL
		// 解析 usage 信息用于按倍率计费
		taskResult.CompletionTokens = resTask.Usage.CompletionTokens
		taskResult.TotalTokens = resTask.Usage.TotalTokens
	case "failed":
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		if resTask.Error != nil {
			taskResult.Reason = fmt.Sprintf("[%s] %s", resTask.Error.Code, resTask.Error.Message)
		} else {
			taskResult.Reason = "task failed"
		}
	case "cancelled":
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		taskResult.Reason = "task cancelled"
	case "expired":
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		taskResult.Reason = "task expired"
	default:
		// Unknown status, treat as processing
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "30%"
	}

	return &taskResult, nil
}

// ConvertToOpenAIVideo 实现 OpenAIVideoConverter 接口
// 将豆包任务数据转换为 OpenAI 视频格式
func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	var doubaoResp responseTask
	if err := json.Unmarshal(task.Data, &doubaoResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal doubao response failed")
	}

	openAIResp := dto.NewOpenAIVideo()
	openAIResp.ID = task.TaskID
	openAIResp.Model = task.Properties.OriginModelName
	openAIResp.SetProgressStr(task.Progress)
	openAIResp.CreatedAt = task.CreatedAt
	openAIResp.CompletedAt = task.UpdatedAt

	// 🔥 优先使用 task.Status（由轮询更新），而不是 doubaoResp.Status（原始响应）
	// task.Status 是由 ParseTaskResult 更新的最新状态
	switch task.Status {
	case model.TaskStatusSuccess:
		openAIResp.Status = dto.VideoStatusCompleted
	case model.TaskStatusFailure:
		openAIResp.Status = dto.VideoStatusFailed
	case model.TaskStatusQueued, model.TaskStatusSubmitted:
		openAIResp.Status = dto.VideoStatusQueued
	case model.TaskStatusInProgress:
		openAIResp.Status = dto.VideoStatusInProgress
	default:
		// 回退到原始响应状态
		openAIResp.Status = convertDoubaoStatus(doubaoResp.Status)
	}

	// 🔥 优先使用 task.FailReason（由轮询更新的视频URL），而不是 doubaoResp.Content.VideoURL
	// relay_task.go 会将视频URL存储在 task.FailReason 中
	videoURL := task.FailReason
	if videoURL == "" {
		videoURL = doubaoResp.Content.VideoURL
	}
	if videoURL != "" {
		openAIResp.SetMetadata("url", videoURL)
	}

	// 设置尾帧URL（如果有）
	if doubaoResp.Content.LastFrameURL != "" {
		openAIResp.SetMetadata("last_frame_url", doubaoResp.Content.LastFrameURL)
	}

	// 设置其他元数据
	if doubaoResp.Resolution != "" {
		openAIResp.Size = doubaoResp.Resolution
	}
	if doubaoResp.Duration > 0 {
		openAIResp.Seconds = fmt.Sprintf("%d", doubaoResp.Duration)
	}

	// 错误处理
	if doubaoResp.Error != nil {
		openAIResp.Error = &dto.OpenAIVideoError{
			Code:    doubaoResp.Error.Code,
			Message: doubaoResp.Error.Message,
		}
	}

	return json.Marshal(openAIResp)
}

// convertDoubaoStatus 将豆包状态转换为 OpenAI 视频状态
func convertDoubaoStatus(doubaoStatus string) string {
	switch doubaoStatus {
	case "queued":
		return dto.VideoStatusQueued
	case "running":
		return dto.VideoStatusInProgress
	case "succeeded":
		return dto.VideoStatusCompleted
	case "failed", "cancelled", "expired":
		return dto.VideoStatusFailed
	default:
		return dto.VideoStatusUnknown
	}
}
