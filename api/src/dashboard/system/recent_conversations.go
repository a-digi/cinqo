package system

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
	conversation_query "github.com/a-digi/cinqo/src/conversation/repository/query"
	"github.com/a-digi/cinqo/src/dashboard"
)

const recentConversationLimit = 5

// RecentConversations is "cinqo.recent_conversations": the caller's own most
// recent AI conversations (hidden ones excluded, like the sidebar). It needs the
// same scope as the conversation routes, and only ever reads the caller's own.
type RecentConversations struct {
	db  *sql.DB
	now func() time.Time
}

func NewRecentConversations(conversationDB *sql.DB) *RecentConversations {
	return &RecentConversations{db: conversationDB, now: time.Now}
}

func (*RecentConversations) Definition() dashboard.Definition {
	return dashboard.Definition{
		Provider:       "cinqo",
		Key:            "recent_conversations",
		Title:          "Recent conversations",
		Description:    "Your latest AI conversations.",
		Kind:           dashboard.KindList,
		Size:           dashboard.Size{W: 2, H: 2},
		RefreshSeconds: dashboard.DefaultRefreshSeconds,
		RequiredScope:  "cinqo:conversation:use",
	}
}

func (r *RecentConversations) Data(_ context.Context, caller dashboard.Caller) ([]byte, error) {
	conversations, err := conversation_query.NewConversationQueryRepo(r.db).ListOwnedBy(caller.UserID)
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	return json.Marshal(recentList(conversations, r.now()))
}

func recentList(conversations []*conversation_entity.Conversation, now time.Time) dashboard.ListData {
	items := []dashboard.ListItem{}
	for _, c := range conversations {
		if len(items) == recentConversationLimit {
			break
		}
		title := c.Title
		if title == "" {
			title = "Untitled conversation"
		}
		items = append(items, dashboard.ListItem{
			Title:    truncate(title, 120),
			Subtitle: truncate(c.Model, 200),
			Meta:     relativeTime(now, c.StartedAt),
			Link:     "/conversations",
		})
	}
	return dashboard.ListData{Items: items}
}

var timeLayouts = []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05"}

// relativeTime renders when, as stored, relative to now: "just now", "5m ago",
// "3h ago", "2d ago", and a plain date once it is over 30 days old. An
// unparseable timestamp gives "" (the row simply shows no age).
func relativeTime(now time.Time, stored string) string {
	var then time.Time
	for _, layout := range timeLayouts {
		if t, err := time.ParseInLocation(layout, stored, time.UTC); err == nil {
			then = t
			break
		}
	}
	if then.IsZero() {
		return ""
	}
	d := now.Sub(then)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d/time.Minute))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d/time.Hour))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d/(24*time.Hour)))
	default:
		return then.Format("2 Jan 2006")
	}
}
