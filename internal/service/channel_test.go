package service_test

import (
	"context"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/channel"
	"github.com/Strob0t/CodeForge/internal/service"
)

// chMockStore overrides channel-related methods on the base runtimeMockStore.
type chMockStore struct {
	runtimeMockStore
	channels []channel.Channel
	messages []channel.Message
	members  []channel.Member
}

func (m *chMockStore) CreateChannel(_ context.Context, ch *channel.Channel) (*channel.Channel, error) {
	ch.ID = "ch-1"
	m.channels = append(m.channels, *ch)
	return ch, nil
}

func (m *chMockStore) GetChannel(_ context.Context, id string) (*channel.Channel, error) {
	for i := range m.channels {
		if m.channels[i].ID == id {
			return &m.channels[i], nil
		}
	}
	return nil, errMockNotFound
}

func (m *chMockStore) ListChannels(_ context.Context, projectID string) ([]channel.Channel, error) {
	if projectID == "" {
		return m.channels, nil
	}
	var result []channel.Channel
	for i := range m.channels {
		if m.channels[i].ProjectID == projectID {
			result = append(result, m.channels[i])
		}
	}
	return result, nil
}

func (m *chMockStore) DeleteChannel(_ context.Context, id string) error {
	for i := range m.channels {
		if m.channels[i].ID == id {
			m.channels = append(m.channels[:i], m.channels[i+1:]...)
			return nil
		}
	}
	return errMockNotFound
}

func (m *chMockStore) CreateChannelMessage(_ context.Context, msg *channel.Message) (*channel.Message, error) {
	msg.ID = "msg-1"
	m.messages = append(m.messages, *msg)
	return msg, nil
}

func (m *chMockStore) ListChannelMessages(_ context.Context, channelID, _ string, limit int) ([]channel.Message, error) {
	var result []channel.Message
	for i := range m.messages {
		if m.messages[i].ChannelID == channelID {
			result = append(result, m.messages[i])
		}
	}
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (m *chMockStore) AddChannelMember(_ context.Context, member *channel.Member) error {
	m.members = append(m.members, *member)
	return nil
}

func (m *chMockStore) UpdateChannelMemberNotify(_ context.Context, channelID, userID string, notify channel.NotifySetting) error {
	for i := range m.members {
		if m.members[i].ChannelID == channelID && m.members[i].UserID == userID {
			m.members[i].Notify = notify
			return nil
		}
	}
	return errMockNotFound
}

func TestChannelService_Create(t *testing.T) {
	tests := []struct {
		name    string
		ch      *channel.Channel
		wantErr bool
	}{
		{
			name: "valid_project",
			ch: &channel.Channel{
				Name:      "general",
				Type:      channel.TypeProject,
				ProjectID: "proj-1",
			},
			wantErr: false,
		},
		{
			name: "valid_bot",
			ch: &channel.Channel{
				Name: "bot-alerts",
				Type: channel.TypeBot,
			},
			wantErr: false,
		},
		{
			name: "empty_name",
			ch: &channel.Channel{
				Name: "",
				Type: channel.TypeProject,
			},
			wantErr: true,
		},
		{
			name: "invalid_type",
			ch: &channel.Channel{
				Name: "test",
				Type: "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &chMockStore{}
			svc := service.NewChannelService(store)
			result, err := svc.Create(context.Background(), tt.ch)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Name != tt.ch.Name {
				t.Errorf("Name = %q, want %q", result.Name, tt.ch.Name)
			}
		})
	}
}

func TestChannelService_Get(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		seed    []channel.Channel
		wantErr bool
	}{
		{
			name: "found",
			id:   "ch-1",
			seed: []channel.Channel{
				{ID: "ch-1", Name: "general", Type: channel.TypeProject},
			},
			wantErr: false,
		},
		{
			name:    "not_found",
			id:      "ch-999",
			seed:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &chMockStore{channels: tt.seed}
			svc := service.NewChannelService(store)
			ch, err := svc.Get(context.Background(), tt.id)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ch.ID != tt.id {
				t.Errorf("ID = %q, want %q", ch.ID, tt.id)
			}
		})
	}
}

func TestChannelService_List(t *testing.T) {
	tests := []struct {
		name      string
		projectID string
		seed      []channel.Channel
		wantCount int
	}{
		{
			name:      "empty",
			projectID: "proj-1",
			seed:      nil,
			wantCount: 0,
		},
		{
			name:      "with_channels",
			projectID: "proj-1",
			seed: []channel.Channel{
				{ID: "ch-1", Name: "general", ProjectID: "proj-1", Type: channel.TypeProject},
				{ID: "ch-2", Name: "alerts", ProjectID: "proj-1", Type: channel.TypeBot},
				{ID: "ch-3", Name: "other", ProjectID: "proj-2", Type: channel.TypeProject},
			},
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &chMockStore{channels: tt.seed}
			svc := service.NewChannelService(store)
			channels, err := svc.List(context.Background(), tt.projectID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(channels) != tt.wantCount {
				t.Errorf("got %d channels, want %d", len(channels), tt.wantCount)
			}
		})
	}
}

func TestChannelService_Delete(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		seed    []channel.Channel
		wantErr bool
	}{
		{
			name: "valid_bot_channel",
			id:   "ch-1",
			seed: []channel.Channel{
				{ID: "ch-1", Name: "bot-alerts", Type: channel.TypeBot},
			},
			wantErr: false,
		},
		{
			name: "project_channel_rejected",
			id:   "ch-2",
			seed: []channel.Channel{
				{ID: "ch-2", Name: "general", Type: channel.TypeProject},
			},
			wantErr: true,
		},
		{
			name:    "not_found",
			id:      "ch-999",
			seed:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &chMockStore{channels: tt.seed}
			svc := service.NewChannelService(store)
			err := svc.Delete(context.Background(), tt.id)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestChannelService_SendMessage(t *testing.T) {
	tests := []struct {
		name    string
		msg     *channel.Message
		wantErr bool
	}{
		{
			name: "valid",
			msg: &channel.Message{
				ChannelID:  "ch-1",
				SenderType: channel.SenderUser,
				SenderName: "alice",
				Content:    "Hello!",
			},
			wantErr: false,
		},
		{
			name: "empty_body",
			msg: &channel.Message{
				ChannelID:  "ch-1",
				SenderType: channel.SenderUser,
				Content:    "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &chMockStore{}
			svc := service.NewChannelService(store)
			msg, err := svc.SendMessage(context.Background(), tt.msg)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if msg.Content != tt.msg.Content {
				t.Errorf("Content = %q, want %q", msg.Content, tt.msg.Content)
			}
		})
	}
}

func TestChannelService_ListMessages(t *testing.T) {
	tests := []struct {
		name      string
		channelID string
		seed      []channel.Message
		wantCount int
	}{
		{
			name:      "empty",
			channelID: "ch-1",
			seed:      nil,
			wantCount: 0,
		},
		{
			name:      "with_messages",
			channelID: "ch-1",
			seed: []channel.Message{
				{ID: "msg-1", ChannelID: "ch-1", Content: "Hello"},
				{ID: "msg-2", ChannelID: "ch-1", Content: "World"},
				{ID: "msg-3", ChannelID: "ch-2", Content: "Other"},
			},
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &chMockStore{messages: tt.seed}
			svc := service.NewChannelService(store)
			messages, err := svc.ListMessages(context.Background(), tt.channelID, "", 50)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(messages) != tt.wantCount {
				t.Errorf("got %d messages, want %d", len(messages), tt.wantCount)
			}
		})
	}
}

func TestChannelService_AddMember(t *testing.T) {
	store := &chMockStore{}
	svc := service.NewChannelService(store)
	err := svc.AddMember(context.Background(), &channel.Member{
		ChannelID: "ch-1",
		UserID:    "user-1",
		Role:      channel.RoleMember,
		Notify:    channel.NotifyAll,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(store.members))
	}
	if store.members[0].UserID != "user-1" {
		t.Errorf("UserID = %q, want %q", store.members[0].UserID, "user-1")
	}
}

func TestChannelService_GenerateWebhookKey(t *testing.T) {
	svc := service.NewChannelService(&chMockStore{})
	key, err := svc.GenerateWebhookKey()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key == "" {
		t.Fatal("expected non-empty key")
	}
	// 32 bytes = 64 hex characters
	if len(key) != 64 {
		t.Errorf("key length = %d, want 64", len(key))
	}

	// Verify uniqueness with a second call.
	key2, err := svc.GenerateWebhookKey()
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if key == key2 {
		t.Error("two consecutive keys should differ")
	}
}
