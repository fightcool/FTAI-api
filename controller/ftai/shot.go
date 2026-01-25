package ftai

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/model/ftai"
	"github.com/gin-gonic/gin"
)

// GetShots 获取项目的镜头列表
func GetShots(c *gin.Context) {
	projectID, err := strconv.ParseInt(c.Param("project_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的项目ID",
		})
		return
	}

	// 验证项目权限
	project, err := ftai.GetProjectByID(projectID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "项目不存在",
		})
		return
	}

	userID := c.GetInt64("id")
	if project.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "无权访问此项目",
		})
		return
	}

	shots, err := ftai.GetShotsByProjectID(projectID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "获取镜头列表失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    shots,
	})
}
