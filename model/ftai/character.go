package ftai

import (
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// Character FTAI角色模型
type Character struct {
	ID          int64          `json:"id" gorm:"primaryKey;autoIncrement"`
	ProjectID   int64          `json:"project_id" gorm:"index;not null"`
	Name        string         `json:"name" gorm:"size:255;not null"`
	Description string         `json:"description" gorm:"type:text"`
	AvatarURL   string         `json:"avatar_url" gorm:"size:500"`
	VoiceID     string         `json:"voice_id" gorm:"size:100"`
	StylePreset string         `json:"style_preset" gorm:"type:json"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index"`
}

func (Character) TableName() string {
	return "ftai_characters"
}

// GetCharactersByProjectID 获取项目的角色列表
func GetCharactersByProjectID(projectID int64) ([]Character, error) {
	var characters []Character
	err := common.DB.Where("project_id = ?", projectID).
		Order("created_at ASC").Find(&characters).Error
	return characters, err
}

// GetCharacterByID 根据ID获取角色
func GetCharacterByID(id int64) (*Character, error) {
	var character Character
	err := common.DB.First(&character, id).Error
	if err != nil {
		return nil, err
	}
	return &character, nil
}

// CreateCharacter 创建角色
func CreateCharacter(character *Character) error {
	return common.DB.Create(character).Error
}

// UpdateCharacter 更新角色
func UpdateCharacter(character *Character) error {
	return common.DB.Save(character).Error
}

// DeleteCharacter 删除角色
func DeleteCharacter(id int64) error {
	return common.DB.Delete(&Character{}, id).Error
}
