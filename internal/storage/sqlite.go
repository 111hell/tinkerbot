package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"

	"github.com/111hell/tinker/agent"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"myagent/internal/chat"
)

var ErrNotFound = chat.ErrNotFound

type conversation struct {
	TopicID  string `gorm:"primaryKey"`
	Messages []byte
}

// agentSession has its own table so legacy raw TopicIDs cannot collide with
// namespaced session IDs from another channel.
type agentSession struct {
	ID       string `gorm:"primaryKey"`
	Messages []byte
}

type SessionStore struct{ db *gorm.DB }

func (s *Store) Sessions() *SessionStore { return &SessionStore{db: s.db} }

func (s *SessionStore) Load(ctx context.Context, id string) ([]agent.Message, error) {
	var stored agentSession
	if err := s.db.WithContext(ctx).First(&stored, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var messages []agent.Message
	if err := json.Unmarshal(stored.Messages, &messages); err != nil {
		return nil, err
	}
	return messages, nil
}

func (s *SessionStore) Save(ctx context.Context, id string, messages []agent.Message) error {
	data, err := json.Marshal(messages)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Save(&agentSession{ID: id, Messages: data}).Error
}

func (s *SessionStore) Delete(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Delete(&agentSession{}, "id = ?", id).Error
}

type Store struct {
	db *gorm.DB
}

func (s *Store) Close() error {
	db, err := s.db.DB()
	if err != nil {
		return err
	}
	return db.Close()
}

func OpenSQLite(path string) (*Store, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve sqlite path: %w", err)
	}
	dsnURL := url.URL{Scheme: "file", Path: absPath}
	dsnURL.RawQuery = url.Values{
		"_busy_timeout": {"5000"},
		"_foreign_keys": {"on"},
		"_journal_mode": {"WAL"},
	}.Encode()
	db, err := gorm.Open(sqlite.Open(dsnURL.String()), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := db.AutoMigrate(&conversation{}, &agentSession{}); err != nil {
		return nil, fmt.Errorf("migrate sqlite: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Load(ctx context.Context, topicID string) ([]agent.Message, error) {
	var stored conversation
	if err := s.db.WithContext(ctx).First(&stored, "topic_id = ?", topicID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var messages []agent.Message
	if err := json.Unmarshal(stored.Messages, &messages); err != nil {
		return nil, err
	}
	return messages, nil
}

func (s *Store) Save(ctx context.Context, topicID string, messages []agent.Message) error {
	data, err := json.Marshal(messages)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Save(&conversation{TopicID: topicID, Messages: data}).Error
}

func (s *Store) Delete(ctx context.Context, topicID string) error {
	return s.db.WithContext(ctx).Delete(&conversation{}, "topic_id = ?", topicID).Error
}
