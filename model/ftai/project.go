package ftai

import (
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// Project FTAI项目模型
type Project struct {
	ID          int64          `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID      int64          `json:"user_id" gorm:"index;not null"`
	Name        string         `json:"name" gorm:"size:255;not null"`
	Description string         `json:"description" gorm:"type:text"`
	CoverImage  string         `json:"cover_image" gorm:"size:500"`
	Settings    string         `json:"settings" gorm:"type:json"`
	Status      string         `json:"status" gorm:"size:50;default:active"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index"`
}

func (Project) TableName() string {
	return "ftai_projects"
}

// GetProjectsByUserID 获取用户的项目列表
func GetProjectsByUserID(userID int64, page, pageSize int) ([]Project, int64, error) {
	var projects []Project
	var total int64

	db := common.DB.Model(&Project{}).Where("user_id = ?", userID)
	db.Count(&total)

	offset := (page - 1) * pageSize
	err := db.Order("updated_at DESC").Offset(offset).Limit(pageSize).Find(&projects).Error
	return projects, total, err
}

// GetProjectByID 根据ID获取项目
func GetProjectByID(id int64) (*Project, error) {
	var project Project
	err := common.DB.First(&project, id).Error
	if err != nil {
		return nil, err
	}
	return &project, nil
}

// CreateProject 创建项目
func CreateProject(project *Project) error {
	return common.DB.Create(project).Error
}

// UpdateProject 更新项目
func UpdateProject(project *Project) error {
	return common.DB.Save(project).Error
}

// DeleteProject 删除项目
func DeleteProject(id int64) error {
	return common.DB.Delete(&Project{}, id).Error
}
