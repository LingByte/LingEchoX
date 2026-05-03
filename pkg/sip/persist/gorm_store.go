package persist

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

type gormCallStore struct{ db *gorm.DB }
type gormUserStore struct{ db *gorm.DB }

// NewGORMStores wires SIP persistence to GORM (tables sip_calls, sip_users). Runs AutoMigrate.
func NewGORMStores(db *gorm.DB) (Stores, error) {
	if db == nil {
		return Stores{}, errors.New("persist: nil db")
	}
	if err := db.AutoMigrate(&SIPCall{}, &SIPUser{}); err != nil {
		return Stores{}, err
	}
	return Stores{
		Calls: &gormCallStore{db: db},
		Users: &gormUserStore{db: db},
	}, nil
}

func (g *gormCallStore) CreateSIPCall(ctx context.Context, c *SIPCall) error {
	if c == nil || c.CallID == "" {
		return errors.New("persist: empty call")
	}
	now := time.Now()
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now
	}
	c.UpdatedAt = now

	var row SIPCall
	err := g.db.WithContext(ctx).Where("call_id = ?", c.CallID).First(&row).Error
	if err == nil {
		MergeSIPCall(&row, c)
		row.UpdatedAt = now
		return g.db.WithContext(ctx).Save(&row).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return g.db.WithContext(ctx).Create(c).Error
}

func (g *gormCallStore) UpdateSIPCall(ctx context.Context, patch *SIPCall) error {
	if patch == nil || patch.CallID == "" {
		return errors.New("persist: empty call_id")
	}
	var row SIPCall
	if err := g.db.WithContext(ctx).Where("call_id = ?", patch.CallID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			now := time.Now()
			row = SIPCall{CallID: patch.CallID, CreatedAt: now}
			MergeSIPCall(&row, patch)
			return g.db.WithContext(ctx).Create(&row).Error
		}
		return err
	}
	MergeSIPCall(&row, patch)
	return g.db.WithContext(ctx).Save(&row).Error
}

func (g *gormUserStore) GetUser(ctx context.Context, username, domain string) (*SIPUser, error) {
	var row SIPUser
	err := g.db.WithContext(ctx).Where("username = ? AND domain = ?", username, domain).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (g *gormUserStore) UpsertUser(ctx context.Context, u *SIPUser) error {
	if u == nil || u.Username == "" {
		return nil
	}
	now := time.Now()
	var existing SIPUser
	err := g.db.WithContext(ctx).Where("username = ? AND domain = ?", u.Username, u.Domain).First(&existing).Error
	if err == nil {
		u.ID = existing.ID
		if u.CreatedAt.IsZero() {
			u.CreatedAt = existing.CreatedAt
		}
		u.UpdatedAt = now
		return g.db.WithContext(ctx).Save(u).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	u.CreatedAt = now
	u.UpdatedAt = now
	return g.db.WithContext(ctx).Create(u).Error
}
