package ftai

import (
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// Shot FTAI镜头模型
type Shot struct {
	ID             int64          `json:"id" gorm:"primaryKey;autoIncrement"`
	ProjectID      int64          `json:"project_id" gorm:"index;not null"`
	SequenceNumber int            `json:"sequence_number" gorm:"not null"`
	Name           string         `json:"name" gorm:"size:255"`
	Prompt         string         `json:"prompt" gorm:"type:text"`
	Narration      string         `json:"narration" gorm:"type:text"`
	CharacterID    *int64         `json:"character_id" gorm:"index"`
	Duration       float64        `json:"duration"`
	AspectRatio    string         `json:"aspect_ratio" gorm:"size:20;default:16:9"`
	VideoURL       string         `json:"video_url" gorm:"size:500"`
	AudioURL       string         `json:"audio_url" gorm:"size:500"`
	ThumbnailURL   string         `json:"thumbnail_url" gorm:"size:500"`
	Status         string         `json:"status" gorm:"size:50;default:pending"`
	Metadata       string         `json:"metadata" gorm:"type:json"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `json:"-" gorm:"index"`
}

func (Shot) TableName() string {
	return "ftai_shots"
}

// GetShotsByProjectID 获取项目的镜头列表
func GetShotsByProjectID(projectID int64) ([]Shot, error) {
	var shots []Shot
	err := common.DB.Where("project_id = ?", projectID).
		Order("sequence_number ASC").Find(&shots).Error
	return shots, err
}

// GetShotByID 根据ID获取镜头
func GetShotByID(id int64) (*Shot, error) {
	var shot Shot
	err := common.DB.First(&shot, id).Error
	if err != nil {
		return nil, err
	}
	return &shot, nil
}

// CreateShot 创建镜头
func CreateShot(shot *Shot) error {
	return common.DB.Create(shot).Error
}

// UpdateShot 更新镜头
func UpdateShot(shot *Shot) error {
	return common.DB.Save(shot).Error
}

// DeleteShot 删除镜头
func DeleteShot(id int64) error {
	return common.DB.Delete(&Shot{}, id).Error
}

// GetMaxSequenceNumber 获取项目中最大的序号
func GetMaxSequenceNumber(projectID int64) (int, error) {
	var maxSeq int
	err := common.DB.Model(&Shot{}).
		Where("project_id = ?", projectID).
		Select("COALESCE(MAX(sequence_number), 0)").
		Scan(&maxSeq).Error
	return maxSeq, err
}
