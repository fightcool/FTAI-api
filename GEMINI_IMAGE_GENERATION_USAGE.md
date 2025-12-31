# Gemini 图像生成功能使用指南

## 概述

new-api 现已支持 Gemini 的 Nano Banana 图像生成功能，包括文生图和图生图两种模式。

## 支持的模型

- `gemini-2.5-flash-image` - Nano Banana（快速图像生成）
- `gemini-3-pro-image-preview` - Nano Banana Pro（专业级 4K 图像生成）
- `gemini-2.0-flash-thinking-exp-image` - 实验性图像生成模型

## API 使用方法

### 1. 文生图（Text-to-Image）

**请求示例：**

```json
{
  "contents": [
    {
      "role": "user",
      "parts": [
        {
          "text": "A beautiful sunset over mountains with vibrant colors"
        }
      ]
    }
  ],
  "generationConfig": {
    "responseModalities": ["IMAGE"],
    "numberOfImages": 1,
    "imageConfig": {
      "aspectRatio": "16:9",
      "imageSize": "4K"
    }
  }
}
```

### 2. 图生图（Image-to-Image）

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
          "text": "Transform this image into anime style"
        }
      ]
    }
  ],
  "generationConfig": {
    "responseModalities": ["IMAGE"],
    "numberOfImages": 1,
    "imageConfig": {
      "aspectRatio": "1:1",
      "imageSize": "4K"
    }
  }
}
```

**重要提示：** 图片必须放在文本之前（系统会自动处理顺序）

## 参数说明

### generationConfig 参数

| 参数 | 类型 | 说明 | 必需 | 默认值 |
|------|------|------|------|--------|
| `responseModalities` | array | 响应模态，图像生成必须为 `["IMAGE"]` | 是 | - |
| `numberOfImages` | number | 生成图片数量（1-4） | 否 | 1 |
| `imageConfig` | object | 图像配置 | 否 | - |

### imageConfig 参数

| 参数 | 类型 | 说明 | 可选值 |
|------|------|------|--------|
| `aspectRatio` | string | 宽高比 | "1:1", "16:9", "9:16", "4:3", "3:4" |
| `imageSize` | string | 图片尺寸 | "4K", "2K", "1K" |

### inlineData 参数（图生图时使用）

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
              "mimeType": "image/png",
              "data": "base64_encoded_generated_image"
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
    "promptTokenCount": 10,
    "candidatesTokenCount": 0,
    "totalTokenCount": 10
  }
}
```

## 使用示例

### cURL 示例

```bash
curl -X POST https://your-api-endpoint/v1beta/models/gemini-3-pro-image-preview:generateContent \
  -H "Content-Type: application/json" \
  -H "x-goog-api-key: YOUR_API_KEY" \
  -d '{
    "contents": [{
      "role": "user",
      "parts": [{
        "text": "A futuristic city at night with neon lights"
      }]
    }],
    "generationConfig": {
      "responseModalities": ["IMAGE"],
      "numberOfImages": 1,
      "imageConfig": {
        "aspectRatio": "16:9",
        "imageSize": "4K"
      }
    }
  }'
```

### Python 示例

```python
import requests
import base64
import json

# 文生图
def text_to_image(prompt, aspect_ratio="1:1", image_size="4K"):
    url = "https://your-api-endpoint/v1beta/models/gemini-3-pro-image-preview:generateContent"
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
            "responseModalities": ["IMAGE"],
            "numberOfImages": 1,
            "imageConfig": {
                "aspectRatio": aspect_ratio,
                "imageSize": image_size
            }
        }
    }

    response = requests.post(url, headers=headers, json=payload)
    return response.json()

# 图生图
def image_to_image(image_path, prompt, aspect_ratio="1:1", image_size="4K"):
    # 读取并编码图片
    with open(image_path, "rb") as f:
        image_data = base64.b64encode(f.read()).decode("utf-8")

    url = "https://your-api-endpoint/v1beta/models/gemini-3-pro-image-preview:generateContent"
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
            "responseModalities": ["IMAGE"],
            "numberOfImages": 1,
            "imageConfig": {
                "aspectRatio": aspect_ratio,
                "imageSize": image_size
            }
        }
    }

    response = requests.post(url, headers=headers, json=payload)
    return response.json()

# 使用示例
result = text_to_image("A beautiful landscape with mountains", "16:9", "4K")
print(json.dumps(result, indent=2))
```

## 注意事项

1. **图片顺序**：在图生图请求中，图片必须放在文本之前（系统会自动处理）
2. **Base64 编码**：参考图片必须使用 Base64 编码
3. **MIME 类型**：支持 `image/png` 和 `image/jpeg`
4. **图片尺寸**：
   - 4K: 最高质量，适合专业用途
   - 2K: 平衡质量和速度
   - 1K: 快速生成
5. **宽高比**：选择合适的宽高比以获得最佳效果

## 故障排查

### 常见错误

1. **错误：responseModalities must include 'IMAGE'**
   - 解决：确保 `generationConfig.responseModalities` 设置为 `["IMAGE"]`

2. **错误：invalid json response**
   - 解决：检查 API 端点是否正确，确保使用 `generateContent` 而不是其他端点

3. **图片无法生成**
   - 检查模型名称是否正确
   - 确认 API 密钥有效
   - 验证请求格式是否符合规范

## 技术实现细节

### 代码修改位置

1. **数据模型** - `dto/gemini.go`
   - 添加 `GeminiImageConfig` 结构体
   - 扩展 `GeminiChatGenerationConfig`

2. **请求处理** - `relay/channel/gemini/adaptor.go`
   - 图像生成请求识别
   - Parts 重排序逻辑

3. **响应处理** - `relay/channel/gemini/relay-gemini.go`
   - 图像生成响应检测
   - 响应格式处理

## 参考文档

- [Gemini API 官方文档 - Nano Banana](https://ai.google.dev/gemini-api/docs/nanobanana?hl=zh-cn)
- [Gemini API 官方文档 - 图像生成](https://ai.google.dev/gemini-api/docs/image-generation?hl=zh-cn)
