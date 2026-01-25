package ftai

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/model/ftai"
	"github.com/gin-gonic/gin"
)

// GetProjects 获取项目列表
func GetProjects(c *gin.Context) {
	userID := c.GetInt64("id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	projects, total, err := ftai.GetProjectsByUserID(userID, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "获取项目列表失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    projects,
		"total":   total,
		"page":    page,
		"pageSize": pageSize,
	})
}

// GetProject 获取项目详情
func GetProject(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的项目ID",
		})
		return
	}

	project, err := ftai.GetProjectByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "项目不存在",
		})
		return
	}

	// 检查权限
	userID := c.GetInt64("id")
	if project.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "无权访问此项目",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    project,
	})
}

// CreateProject 创建项目
func CreateProject(c *gin.Context) {
	userID := c.GetInt64("id")

	var req struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
		CoverImage  string `json:"cover_image"`
		Settings    string `json:"settings"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "参数错误: " + err.Error(),
		})
		return
	}

	project := &ftai.Project{
		UserID:      userID,
		Name:        req.Name,
		Description: req.Description,
		CoverImage:  req.CoverImage,
		Settings:    req.Settings,
		Status:      "active",
	}

	if err := ftai.CreateProject(project); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "创建项目失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    project,
	})
}

// UpdateProject 更新项目
func UpdateProject(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的项目ID",
		})
		return
	}

	project, err := ftai.GetProjectByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "项目不存在",
		})
		return
	}

	// 检查权限
	userID := c.GetInt64("id")
	if project.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "无权修改此项目",
		})
		return
	}

	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
		CoverImage  *string `json:"cover_image"`
		Settings    *string `json:"settings"`
		Status      *string `json:"status"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}

	if req.Name != nil {
		project.Name = *req.Name
	}
	if req.Description != nil {
		project.Description = *req.Description
	}
	if req.CoverImage != nil {
		project.CoverImage = *req.CoverImage
	}
	if req.Settings != nil {
		project.Settings = *req.Settings
	}
	if req.Status != nil {
		project.Status = *req.Status
	}

	if err := ftai.UpdateProject(project); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "更新项目失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    project,
	})
}

// DeleteProject 删除项目
func DeleteProject(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的项目ID",
		})
		return
	}

	project, err := ftai.GetProjectByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "项目不存在",
		})
		return
	}

	// 检查权限
	userID := c.GetInt64("id")
	if project.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "无权删除此项目",
		})
		return
	}

	if err := ftai.DeleteProject(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "删除项目失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "项目已删除",
	})
}
