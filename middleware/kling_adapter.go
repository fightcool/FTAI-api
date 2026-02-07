package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"

	"github.com/gin-gonic/gin"
)

func KlingRequestConvert() func(c *gin.Context) {
	return func(c *gin.Context) {
		var originalReq map[string]interface{}
		if err := common.UnmarshalBodyReusable(c, &originalReq); err != nil {
			c.Next()
			return
		}

		// Support both model_name and model fields
		model, _ := originalReq["model_name"].(string)
		if model == "" {
			model, _ = originalReq["model"].(string)
		}
		prompt, _ := originalReq["prompt"].(string)

		unifiedReq := map[string]interface{}{
			"model":    model,
			"prompt":   prompt,
			"metadata": originalReq,
		}

		jsonData, err := json.Marshal(unifiedReq)
		if err != nil {
			c.Next()
			return
		}

		// Rewrite request body and path
		c.Request.Body = io.NopCloser(bytes.NewBuffer(jsonData))
		c.Request.URL.Path = "/v1/video/generations"
		if image, ok := originalReq["image"]; !ok || image == "" {
			c.Set("action", constant.TaskActionTextGenerate)
		}

		// We have to reset the request body for the next handlers
		c.Set(common.KeyRequestBody, jsonData)
		c.Next()
	}
}

// KlingLipSyncConvert 对口型专用中间件
// 将 Kling 原生对口型请求转换为统一格式，保留原始请求体在 metadata 中
// 支持人脸识别 (identify-face) 和对口型生成 (advanced-lip-sync)
func KlingLipSyncConvert() func(c *gin.Context) {
	return func(c *gin.Context) {
		var originalReq map[string]interface{}
		if err := common.UnmarshalBodyReusable(c, &originalReq); err != nil {
			c.Next()
			return
		}

		// 从请求中提取 model（对口型请求可能包含 model 字段）
		model, _ := originalReq["model"].(string)
		if model == "" {
			model, _ = originalReq["model_name"].(string)
		}

		// 判断是人脸识别还是对口型生成
		path := c.Request.URL.Path
		isIdentifyFace := strings.Contains(path, "identify-face")

		// 设置默认 prompt（对口型请求不需要 prompt，但统一格式需要）
		prompt := "lip-sync"
		if isIdentifyFace {
			prompt = "identify-face"
		}

		// 构造统一格式：model + prompt + metadata（原始请求体）
		unifiedReq := map[string]interface{}{
			"model":    model,
			"prompt":   prompt,
			"metadata": originalReq,
		}

		// 如果有 video_url，放入 image 字段（兼容 TaskSubmitReq.Image）
		if videoUrl, ok := originalReq["video_url"].(string); ok && videoUrl != "" {
			unifiedReq["image"] = videoUrl
		}

		jsonData, err := json.Marshal(unifiedReq)
		if err != nil {
			c.Next()
			return
		}

		// 重写请求体，路径改为统一的视频生成入口
		c.Request.Body = io.NopCloser(bytes.NewBuffer(jsonData))
		c.Request.URL.Path = "/v1/video/generations"

		// 设置对口型 action（区分人脸识别和对口型生成）
		if isIdentifyFace {
			c.Set("action", constant.TaskActionIdentifyFace)
		} else {
			c.Set("action", constant.TaskActionLipSync)
		}

		c.Set(common.KeyRequestBody, jsonData)
		c.Next()
	}
}
