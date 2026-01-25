package ftai

import (
	"time"

	"github.com/QuantumNous/new-api/common"
)

// Asset FTAI资产模型
type Asset struct {
	ID        int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	ProjectID *int64    `json:"project_id" gorm:"index"`
	UserID    int64     `json:"user_id" gorm:"index;not null"`
	Type      string    `json:"type" gorm:"size:50;not null"`
	Name      string    `json:"name" gorm:"size:255"`
	URL       string    `json:"url" gorm:"size:500;not null"`
	OSSKey    string    `json:"oss_key" gorm:"size:500"`
	FileSize  int64     `json:"file_size"`
	MimeType  string    `json:"mime_type" gorm:"size:100"`
	Metadata  string    `json:"metadata" gorm:"type:json"`
	CreatedAt time.Time `json:"created_at"`
}

func (Asset) TableName() string {
	return "ftai_assets"
}

// GetAssetsByUserID 获取用户的资产列表
func GetAssetsByUserID(userID int64, assetType string, page, pageSize int) ([]Asset, int64, error) {
	var assets []Asset
	var total int64

	db := common.DB.Model(&Asset{}).Where("user_id = ?", userID)
	if assetType != "" {
		db = db.Where("type = ?", assetType)
	}
	db.Count(&total)

	offset := (page - 1) * pageSize
	err := db.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&assets).Error
	return assets, total, err
}

// GetAssetByID 根据ID获取资产
func GetAssetByID(id int64) (*Asset, error) {
	var asset Asset
	err := common.DB.First(&asset, id).Error
	if err != nil {
		return nil, err
	}
	return &asset, nil
}

// CreateAsset 创建资产
func CreateAsset(asset *Asset) error {
	return common.DB.Create(asset).Error
}

// DeleteAsset 删除资产
func DeleteAsset(id int64) error {
	return common.DB.Delete(&Asset{}, id).Error
}
