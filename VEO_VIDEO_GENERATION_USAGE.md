# Veo 3.1 视频生成功能使用指南

## 概述

new-api 现已支持 Gemini 的 Veo 3.1 视频生成功能，包括文生视频和图生视频两种模式。

## 支持的模型

- `veo-3.1-generate-preview` - Veo 3.1（最先进的视频生成模型，支持原生音频）
- `veo-3.1-fast-generate-preview` - Veo 3.1 Fast（快速视频生成）
- `veo-3.0-generate-001` - Veo 3.0
- `veo-2.0-generate-001` - Veo 2.0

## API 使用方法

### 1. 文生视频（Text-to-Video）

**请求示例：**

```json
{
  "contents": [
    {
      "role": "user",
      "parts": [
        {
          "text": "A serene beach at sunset with waves gently crashing on the shore"
        }
      ]
    }
  ],
  "generationConfig": {
    "responseModalities": ["VIDEO"],
    "videoConfig": {
      "aspectRatio": "16:9",
      "resolution": "720p",
      "durationSeconds": "8"
    }
  }
}
```

### 2. 图生视频（Image-to-Video）

**请求示例：**

```json
{
  "contents": [
    {
      "role": "user",
      "parts": [
        {
          "inlineData": {
            "mimeType": "image/png",
            "data": "base64_encoded_image_data_here"
          }
        },
        {
          "text": "Animate this image with gentle camera movement"
        }
      ]
    }
  ],
  "generationConfig": {
    "responseModalities": ["VIDEO"],
    "videoConfig": {
      "aspectRatio": "16:9",
      "resolution": "1080p",
      "durationSeconds": "6"
    }
  }
}
```

**重要提示：** 图片必须放在文本之前（系统会自动处理顺序）

## 参数说明

### generationConfig 参数

| 参数 | 类型 | 说明 | 必需 | 默认值 |
|------|------|------|------|--------|
| `responseModalities` | array | 响应模态，视频生成必须为 `["VIDEO"]` | 是 | - |
| `videoConfig` | object | 视频配置 | 否 | 见下表 |

### videoConfig 参数

| 参数 | 类型 | 说明 | 可选值 | 默认值 |
|------|------|------|--------|--------|
| `aspectRatio` | string | 宽高比 | "16:9", "9:16" | "16:9" |
| `resolution` | string | 分辨率 | "720p", "1080p" | "720p" |
| `durationSeconds` | string | 视频时长（秒） | "4", "6", "8" | "8" |

### inlineData 参数（图生视频时使用）

| 参数 | 类型 | 说明 |
|------|------|------|
| `mimeType` | string | 图片 MIME 类型（"image/png", "image/jpeg"） |
| `data` | string | Base64 编码的图片数据 |

## 响应格式

响应将返回 Gemini 原生格式：

```json
{
  "candidates": [
    {
      "content": {
        "parts": [
          {
            "inlineData": {
              "mimeType": "video/mp4",
              "data": "base64_encoded_generated_video"
            }
          }
        ],
        "role": "model"
      },
      "finishReason": "STOP",
      "index": 0
    }
  ],
  "usageMetadata": {
    "promptTokenCount": 15,
    "candidatesTokenCount": 0,
    "totalTokenCount": 15
  }
}
```

## 使用示例

### cURL 示例

```bash
curl -X POST https://your-api-endpoint/v1beta/models/veo-3.1-generate-preview:generateContent \
  -H "Content-Type: application/json" \
  -H "x-goog-api-key: YOUR_API_KEY" \
  -d '{
    "contents": [{
      "role": "user",
      "parts": [{
        "text": "A futuristic city at night with flying cars and neon lights"
      }]
    }],
    "generationConfig": {
      "responseModalities": ["VIDEO"],
      "videoConfig": {
        "aspectRatio": "16:9",
        "resolution": "720p",
        "durationSeconds": "8"
      }
    }
  }'
```

### Python 示例

```python
import requests
import base64
import json

# 文生视频
def text_to_video(prompt, aspect_ratio="16:9", resolution="720p", duration="8"):
    url = "https://your-api-endpoint/v1beta/models/veo-3.1-generate-preview:generateContent"
    headers = {
        "Content-Type": "application/json",
        "x-goog-api-key": "YOUR_API_KEY"
    }

    payload = {
        "contents": [{
            "role": "user",
            "parts": [{"text": prompt}]
        }],
        "generationConfig": {
            "responseModalities": ["VIDEO"],
            "videoConfig": {
                "aspectRatio": aspect_ratio,
                "resolution": resolution,
                "durationSeconds": duration
            }
        }
    }

    response = requests.post(url, headers=headers, json=payload)
    return response.json()

# 图生视频
def image_to_video(image_path, prompt, aspect_ratio="16:9", resolution="720p", duration="6"):
    # 读取并编码图片
    with open(image_path, "rb") as f:
        image_data = base64.b64encode(f.read()).decode("utf-8")

    url = "https://your-api-endpoint/v1beta/models/veo-3.1-generate-preview:generateContent"
    headers = {
        "Content-Type": "application/json",
        "x-goog-api-key": "YOUR_API_KEY"
    }

    payload = {
        "contents": [{
            "role": "user",
            "parts": [
                {
                    "inlineData": {
                        "mimeType": "image/png",
                        "data": image_data
                    }
                },
                {"text": prompt}
            ]
        }],
        "generationConfig": {
            "responseModalities": ["VIDEO"],
            "videoConfig": {
                "aspectRatio": aspect_ratio,
                "resolution": resolution,
                "durationSeconds": duration
            }
        }
    }

    response = requests.post(url, headers=headers, json=payload)
    return response.json()

# 使用示例
result = text_to_video("A beautiful landscape with mountains and rivers", "16:9", "720p", "8")
print(json.dumps(result, indent=2))
```

## 注意事项

1. **图片顺序**：在图生视频请求中，图片必须放在文本之前（系统会自动处理）
2. **Base64 编码**：参考图片必须使用 Base64 编码
3. **MIME 类型**：支持 `image/png` 和 `image/jpeg`
4. **视频规格**：
   - 最大时长：8 秒
   - 帧率：24 fps
   - 分辨率：720p 或 1080p（1080p 限制为 8 秒）
   - 宽高比：16:9 或 9:16
5. **原生音频**：Veo 3.1 支持原生音频生成
6. **SynthID 水印**：所有生成的视频都会自动添加 SynthID 数字水印
7. **存储时间**：生成的视频在服务器上保留 2 天

## 故障排查

### 常见错误

1. **错误：responseModalities must include 'VIDEO'**
   - 解决：确保 `generationConfig.responseModalities` 设置为 `["VIDEO"]`

2. **错误：invalid json response**
   - 解决：检查 API 端点是否正确，确保使用 `generateContent` 而不是其他端点

3. **视频无法生成**
   - 检查模型名称是否正确（必须以 `veo-` 开头）
   - 确认 API 密钥有效
   - 验证请求格式是否符合规范

## 技术实现细节

### 代码修改位置

1. **数据模型** - `dto/gemini.go`
   - 添加 `GeminiVideoConfig` 结构体
   - 扩展 `GeminiChatGenerationConfig` 添加 `VideoConfig` 字段

2. **请求处理** - `relay/channel/gemini/adaptor.go`
   - 视频生成请求识别（`isVideoGenerationRequest`）
   - 视频生成请求处理（`handleVideoGenerationRequest`）
   - Parts 重排序逻辑（图生视频时）

3. **响应处理** - `relay/channel/gemini/relay-gemini.go`
   - 视频生成响应检测（`isVideoGenerationResponse`）
   - 响应格式处理（`handleVideoGenerationResponse`）

## 参考文档

- [Gemini API 官方文档 - Veo 视频生成](https://ai.google.dev/gemini-api/docs/video?hl=zh-cn)
- [Google AI for Developers](https://ai.google.dev)

## 功能特性

### Veo 3.1 核心能力

1. **高保真视频生成**：支持 720p 和 1080p 分辨率
2. **原生音频**：自动生成与视频同步的音频
3. **图生视频**：从静态图片生成动态视频
4. **多种时长**：支持 4、6、8 秒视频
5. **灵活宽高比**：支持 16:9 和 9:16
6. **SynthID 水印**：自动添加数字水印标识 AI 生成内容
