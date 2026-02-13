package gemini

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/reasoning"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

type Adaptor struct {
}

func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	// Check if this is a video generation request
	if isVideoGenerationRequest(request) {
		err := handleVideoGenerationRequest(request)
		if err != nil {
			return nil, err
		}
	}

	// Check if this is an image generation request
	if isImageGenerationRequest(request) {
		err := handleImageGenerationRequest(request)
		if err != nil {
			return nil, err
		}

		// Log request details for debugging
		if len(request.Contents) > 0 && len(request.Contents[0].Parts) > 0 {
			logger.LogDebug(c, fmt.Sprintf("Image generation request with %d parts", len(request.Contents[0].Parts)))
			for i, part := range request.Contents[0].Parts {
				if part.InlineData != nil {
					logger.LogDebug(c, fmt.Sprintf("Part %d: InlineData (mimeType: %s, size: %d bytes)", i, part.InlineData.MimeType, len(part.InlineData.Data)))
				} else if part.Text != "" {
					logger.LogDebug(c, fmt.Sprintf("Part %d: Text (length: %d)", i, len(part.Text)))
				}
			}
		}
	}

	if len(request.Contents) > 0 {
		for i, content := range request.Contents {
			if i == 0 {
				if request.Contents[0].Role == "" {
					request.Contents[0].Role = "user"
				}
			}
			for _, part := range content.Parts {
				if part.FileData != nil {
					if part.FileData.MimeType == "" && strings.Contains(part.FileData.FileUri, "www.youtube.com") {
						part.FileData.MimeType = "video/webm"
					}
				}
			}
		}
	}
	return request, nil
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, req *dto.ClaudeRequest) (any, error) {
	adaptor := openai.Adaptor{}
	oaiReq, err := adaptor.ConvertClaudeRequest(c, info, req)
	if err != nil {
		return nil, err
	}
	return a.ConvertOpenAIRequest(c, info, oaiReq.(*dto.GeneralOpenAIRequest))
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	// Check if this is a Gemini image generation model (gemini-2.5-flash-image, gemini-3-pro-image, etc.)
	if isImageGenerationModel(info.UpstreamModelName) {
		return a.convertImageRequestToGeminiChat(c, info, request)
	}

	// For imagen models, use the original predict endpoint format
	if !strings.HasPrefix(info.UpstreamModelName, "imagen") {
		return nil, errors.New("not supported model for image generation")
	}

	// convert size to aspect ratio but allow user to specify aspect ratio
	aspectRatio := "1:1" // default aspect ratio
	size := strings.TrimSpace(request.Size)
	if size != "" {
		if strings.Contains(size, ":") {
			aspectRatio = size
		} else {
			switch size {
			case "256x256", "512x512", "1024x1024":
				aspectRatio = "1:1"
			case "1536x1024":
				aspectRatio = "3:2"
			case "1024x1536":
				aspectRatio = "2:3"
			case "1024x1792":
				aspectRatio = "9:16"
			case "1792x1024":
				aspectRatio = "16:9"
			}
		}
	}

	// build gemini imagen request
	geminiRequest := dto.GeminiImageRequest{
		Instances: []dto.GeminiImageInstance{
			{
				Prompt: request.Prompt,
			},
		},
		Parameters: dto.GeminiImageParameters{
			SampleCount:      int(request.N),
			AspectRatio:      aspectRatio,
			PersonGeneration: "allow_adult", // default allow adult
		},
	}

	// Set imageSize when quality parameter is specified
	// Map quality parameter to imageSize (only supported by Standard and Ultra models)
	// quality values: auto, high, medium, low (for gpt-image-1), hd, standard (for dall-e-3)
	// imageSize values: 1K (default), 2K
	// https://ai.google.dev/gemini-api/docs/imagen
	// https://platform.openai.com/docs/api-reference/images/create
	if request.Quality != "" {
		imageSize := "1K" // default
		switch request.Quality {
		case "hd", "high":
			imageSize = "2K"
		case "2K":
			imageSize = "2K"
		case "standard", "medium", "low", "auto", "1K":
			imageSize = "1K"
		default:
			// unknown quality value, default to 1K
			imageSize = "1K"
		}
		geminiRequest.Parameters.ImageSize = imageSize
	}

	return geminiRequest, nil
}

// convertImageRequestToGeminiChat converts OpenAI ImageRequest to Gemini generateContent format
// for Gemini image generation models (gemini-2.5-flash-image, gemini-3-pro-image, etc.)
func (a *Adaptor) convertImageRequestToGeminiChat(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	// Parse aspect ratio from size parameter
	// Gemini NanoBanana supports 10 aspect ratios:
	// Landscape: 21:9, 16:9, 4:3, 3:2
	// Square: 1:1
	// Portrait: 9:16, 3:4, 2:3
	// Flexible: 5:4, 4:5
	aspectRatio := "1:1" // default
	size := strings.TrimSpace(request.Size)
	if size != "" {
		if strings.Contains(size, ":") {
			// Direct aspect ratio format (e.g., "16:9")
			aspectRatio = size
		} else {
			// Pixel dimension format (e.g., "1920x1080")
			switch size {
			// Square 1:1
			case "256x256", "512x512", "1024x1024", "2048x2048", "4096x4096":
				aspectRatio = "1:1"
			// Landscape 16:9
			case "1792x1024", "1920x1080", "1280x720", "3840x2160", "7680x4320":
				aspectRatio = "16:9"
			// Portrait 9:16
			case "1024x1792", "1080x1920", "720x1280", "2160x3840", "4320x7680":
				aspectRatio = "9:16"
			// Landscape 4:3
			case "1024x768", "1280x960", "1600x1200", "2880x2160", "5760x4320":
				aspectRatio = "4:3"
			// Portrait 3:4
			case "768x1024", "960x1280", "1200x1600", "2160x2880", "4320x5760":
				aspectRatio = "3:4"
			// Landscape 3:2
			case "1536x1024", "1440x960", "1800x1200", "3240x2160", "6480x4320":
				aspectRatio = "3:2"
			// Portrait 2:3
			case "1024x1536", "960x1440", "1200x1800", "2160x3240", "4320x6480":
				aspectRatio = "2:3"
			// Landscape 21:9 (ultrawide)
			case "2560x1080", "3440x1440", "3840x1646", "7680x3291":
				aspectRatio = "21:9"
			// Flexible 5:4
			case "1280x1024", "1600x1280", "2700x2160", "5400x4320":
				aspectRatio = "5:4"
			// Flexible 4:5
			case "1024x1280", "1280x1600", "2160x2700", "4320x5400":
				aspectRatio = "4:5"
			default:
				// Fallback: calculate aspect ratio from pixel dimensions using GCD
				if parts := strings.SplitN(size, "x", 2); len(parts) == 2 {
					w, errW := strconv.Atoi(parts[0])
					h, errH := strconv.Atoi(parts[1])
					if errW == nil && errH == nil && w > 0 && h > 0 {
						g := gcd(w, h)
						aspectRatio = fmt.Sprintf("%d:%d", w/g, h/g)
					}
				}
			}
		}
	}

	// Map quality to imageSize
	// Gemini supports: "1K", "2K", "4K"
	imageSize := "1K" // default
	if request.Quality != "" {
		switch request.Quality {
		case "hd", "high", "4K", "8K":
			imageSize = "4K"
		case "2K":
			imageSize = "2K"
		case "standard", "medium", "low", "auto", "1K":
			imageSize = "1K"
		default:
			imageSize = "1K"
		}
	}

	// Build parts for the request
	var parts []dto.GeminiPart

	// Helper function to parse image data URL
	parseImageData := func(imageData string) (mimeType string, base64Data string) {
		mimeType = "image/png"
		base64Data = imageData

		if strings.HasPrefix(imageData, "data:") {
			urlParts := strings.SplitN(imageData, ",", 2)
			if len(urlParts) == 2 {
				base64Data = urlParts[1]
				if strings.Contains(urlParts[0], "image/jpeg") {
					mimeType = "image/jpeg"
				} else if strings.Contains(urlParts[0], "image/webp") {
					mimeType = "image/webp"
				} else if strings.Contains(urlParts[0], "image/gif") {
					mimeType = "image/gif"
				}
			}
		}
		return
	}

	// Helper function to add image part
	addImagePart := func(imageData string) {
		mimeType, base64Data := parseImageData(imageData)
		parts = append(parts, dto.GeminiPart{
			InlineData: &dto.GeminiInlineData{
				MimeType: mimeType,
				Data:     base64Data,
			},
		})
	}

	// Handle reference images for image-to-image generation
	// Priority: images array > single image field
	if imagesRaw, ok := request.Extra["images"]; ok && len(imagesRaw) > 0 {
		// Multiple reference images (gemini-3-pro-image-preview supports up to 14 images)
		var images []string
		if err := common.Unmarshal(imagesRaw, &images); err == nil && len(images) > 0 {
			for i, img := range images {
				if i >= 14 {
					break // Gemini supports max 14 reference images
				}
				addImagePart(img)
			}
			logger.LogDebug(c, fmt.Sprintf("Added %d reference images from 'images' field", len(images)))
		}
	} else if len(request.Image) > 0 {
		// Single reference image
		var imageData string
		if err := common.Unmarshal(request.Image, &imageData); err == nil && imageData != "" {
			addImagePart(imageData)
			logger.LogDebug(c, "Added single reference image from 'image' field")
		}
	}

	// Add text prompt
	parts = append(parts, dto.GeminiPart{
		Text: request.Prompt,
	})

	// Build Gemini generateContent request
	geminiRequest := dto.GeminiChatRequest{
		Contents: []dto.GeminiChatContent{
			{
				Role:  "user",
				Parts: parts,
			},
		},
		GenerationConfig: dto.GeminiChatGenerationConfig{
			ResponseModalities: []string{"TEXT", "IMAGE"},
			ImageConfig: &dto.GeminiImageConfig{
				AspectRatio: aspectRatio,
				ImageSize:   imageSize,
			},
		},
	}

	// Mark this as a Gemini image generation request for response handling
	info.IsGeminiImageGeneration = true

	logger.LogDebug(c, fmt.Sprintf("Converted OpenAI ImageRequest to Gemini generateContent: model=%s, aspectRatio=%s, imageSize=%s",
		info.UpstreamModelName, aspectRatio, imageSize))

	return geminiRequest, nil
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {

}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {

	if model_setting.GetGeminiSettings().ThinkingAdapterEnabled &&
		!model_setting.ShouldPreserveThinkingSuffix(info.OriginModelName) {
		// 新增逻辑：处理 -thinking-<budget> 格式
		if strings.Contains(info.UpstreamModelName, "-thinking-") {
			parts := strings.Split(info.UpstreamModelName, "-thinking-")
			info.UpstreamModelName = parts[0]
		} else if strings.HasSuffix(info.UpstreamModelName, "-thinking") { // 旧的适配
			info.UpstreamModelName = strings.TrimSuffix(info.UpstreamModelName, "-thinking")
		} else if strings.HasSuffix(info.UpstreamModelName, "-nothinking") {
			info.UpstreamModelName = strings.TrimSuffix(info.UpstreamModelName, "-nothinking")
		} else if baseModel, level, ok := reasoning.TrimEffortSuffix(info.UpstreamModelName); ok && level != "" {
			info.UpstreamModelName = baseModel
		}
	}

	version := model_setting.GetGeminiVersionSetting(info.UpstreamModelName)

	if strings.HasPrefix(info.UpstreamModelName, "imagen") {
		return fmt.Sprintf("%s/%s/models/%s:predict", info.ChannelBaseUrl, version, info.UpstreamModelName), nil
	}

	if strings.HasPrefix(info.UpstreamModelName, "text-embedding") ||
		strings.HasPrefix(info.UpstreamModelName, "embedding") ||
		strings.HasPrefix(info.UpstreamModelName, "gemini-embedding") {
		action := "embedContent"
		if info.IsGeminiBatchEmbedding {
			action = "batchEmbedContents"
		}
		return fmt.Sprintf("%s/%s/models/%s:%s", info.ChannelBaseUrl, version, info.UpstreamModelName, action), nil
	}

	action := "generateContent"
	if info.IsStream {
		action = "streamGenerateContent?alt=sse"
		if info.RelayMode == constant.RelayModeGemini {
			info.DisablePing = true
		}
	}
	return fmt.Sprintf("%s/%s/models/%s:%s", info.ChannelBaseUrl, version, info.UpstreamModelName, action), nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("x-goog-api-key", info.ApiKey)
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}

	geminiRequest, err := CovertOpenAI2Gemini(c, *request, info)
	if err != nil {
		return nil, err
	}

	return geminiRequest, nil
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	if request.Input == nil {
		return nil, errors.New("input is required")
	}

	inputs := request.ParseInput()
	if len(inputs) == 0 {
		return nil, errors.New("input is empty")
	}
	// We always build a batch-style payload with `requests`, so ensure we call the
	// batch endpoint upstream to avoid payload/endpoint mismatches.
	info.IsGeminiBatchEmbedding = true
	// process all inputs
	geminiRequests := make([]map[string]interface{}, 0, len(inputs))
	for _, input := range inputs {
		geminiRequest := map[string]interface{}{
			"model": fmt.Sprintf("models/%s", info.UpstreamModelName),
			"content": dto.GeminiChatContent{
				Parts: []dto.GeminiPart{
					{
						Text: input,
					},
				},
			},
		}

		// set specific parameters for different models
		// https://ai.google.dev/api/embeddings?hl=zh-cn#method:-models.embedcontent
		switch info.UpstreamModelName {
		case "text-embedding-004", "gemini-embedding-exp-03-07", "gemini-embedding-001":
			// Only newer models introduced after 2024 support OutputDimensionality
			if request.Dimensions > 0 {
				geminiRequest["outputDimensionality"] = request.Dimensions
			}
		}
		geminiRequests = append(geminiRequests, geminiRequest)
	}

	return map[string]interface{}{
		"requests": geminiRequests,
	}, nil
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	// TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	if info.RelayMode == constant.RelayModeGemini {
		if strings.Contains(info.RequestURLPath, ":embedContent") ||
			strings.Contains(info.RequestURLPath, ":batchEmbedContents") {
			return NativeGeminiEmbeddingHandler(c, resp, info)
		}
		if info.IsStream {
			return GeminiTextGenerationStreamHandler(c, info, resp)
		} else {
			return GeminiTextGenerationHandler(c, info, resp)
		}
	}

	if strings.HasPrefix(info.UpstreamModelName, "imagen") {
		return GeminiImageHandler(c, info, resp)
	}

	// Handle Gemini image generation models (gemini-2.5-flash-image, gemini-3-pro-image, etc.)
	if info.IsGeminiImageGeneration {
		return GeminiChatImageHandler(c, info, resp)
	}

	// check if the model is an embedding model
	if strings.HasPrefix(info.UpstreamModelName, "text-embedding") ||
		strings.HasPrefix(info.UpstreamModelName, "embedding") ||
		strings.HasPrefix(info.UpstreamModelName, "gemini-embedding") {
		return GeminiEmbeddingHandler(c, info, resp)
	}

	if info.IsStream {
		return GeminiChatStreamHandler(c, info, resp)
	} else {
		return GeminiChatHandler(c, info, resp)
	}

}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}

// isImageGenerationModel checks if the model supports image generation (Nano Banana models)
func isImageGenerationModel(modelName string) bool {
	return strings.HasPrefix(modelName, "gemini-2.5-flash-image") ||
		strings.HasPrefix(modelName, "gemini-3-pro-image") ||
		strings.HasPrefix(modelName, "gemini-2.0-flash-thinking-exp-image")
}

// isVideoGenerationModel checks if the model supports video generation (Veo models)
func isVideoGenerationModel(modelName string) bool {
	return strings.HasPrefix(modelName, "veo-")
}

// isVideoGenerationRequest checks if the request is for video generation
func isVideoGenerationRequest(request *dto.GeminiChatRequest) bool {
	if request.GenerationConfig.ResponseModalities == nil {
		return false
	}

	// Check if responseModalities contains "VIDEO"
	for _, modality := range request.GenerationConfig.ResponseModalities {
		if modality == "VIDEO" {
			return true
		}
	}

	return false
}

// isImageGenerationRequest checks if the request is for image generation
func isImageGenerationRequest(request *dto.GeminiChatRequest) bool {
	if request.GenerationConfig.ResponseModalities == nil {
		return false
	}

	// Check if responseModalities contains "IMAGE"
	for _, modality := range request.GenerationConfig.ResponseModalities {
		if modality == "IMAGE" {
			return true
		}
	}

	return false
}

// handleImageGenerationRequest processes image generation requests
func handleImageGenerationRequest(request *dto.GeminiChatRequest) error {
	// 1. Validate required parameters
	if !isImageGenerationRequest(request) {
		return errors.New("responseModalities must include 'IMAGE' for image generation")
	}

	// 2. Remove numberOfImages as it's not supported by Gemini API
	request.GenerationConfig.NumberOfImages = 0

	// 3. Reorder parts: images first, then text (required by Gemini API)
	if len(request.Contents) > 0 && len(request.Contents[0].Parts) > 1 {
		reorderPartsForImageGeneration(&request.Contents[0])
	}

	return nil
}

// reorderPartsForImageGeneration reorders parts to put images before text
func reorderPartsForImageGeneration(content *dto.GeminiChatContent) {
	var imageParts []dto.GeminiPart
	var textParts []dto.GeminiPart

	// Separate image and text parts
	for _, part := range content.Parts {
		if part.InlineData != nil {
			imageParts = append(imageParts, part)
		} else {
			textParts = append(textParts, part)
		}
	}

	// Reorder: images first, then text
	content.Parts = append(imageParts, textParts...)
}

// handleVideoGenerationRequest processes video generation requests
func handleVideoGenerationRequest(request *dto.GeminiChatRequest) error {
	// 1. Validate required parameters
	if !isVideoGenerationRequest(request) {
		return errors.New("responseModalities must include 'VIDEO' for video generation")
	}

	// 2. Set default values
	if request.GenerationConfig.VideoConfig == nil {
		request.GenerationConfig.VideoConfig = &dto.GeminiVideoConfig{}
	}

	// Set default aspect ratio if not specified
	if request.GenerationConfig.VideoConfig.AspectRatio == "" {
		request.GenerationConfig.VideoConfig.AspectRatio = "16:9"
	}

	// Set default duration if not specified
	if request.GenerationConfig.VideoConfig.DurationSeconds == "" {
		request.GenerationConfig.VideoConfig.DurationSeconds = "8"
	}

	// Set default resolution if not specified
	if request.GenerationConfig.VideoConfig.Resolution == "" {
		request.GenerationConfig.VideoConfig.Resolution = "720p"
	}

	// 3. Reorder parts: images first, then text (for image-to-video)
	if len(request.Contents) > 0 && len(request.Contents[0].Parts) > 1 {
		reorderPartsForImageGeneration(&request.Contents[0])
	}

	return nil
}

// gcd calculates the Greatest Common Divisor using Euclidean algorithm.
// Used for dynamic aspect ratio calculation from pixel dimensions.
func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}
