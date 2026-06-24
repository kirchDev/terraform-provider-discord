package provider

import (
	"context"

	"github.com/kirchDev/terraform-provider-discord/internal/client"
)

// Discord channel type ids. A guild channel's kind is the integer `type`; the
// provider exposes a typed resource per kind and always sends the right one.
const (
	channelTypeText     = 0
	channelTypeVoice    = 2
	channelTypeCategory = 4
	channelTypeNews     = 5  // GUILD_ANNOUNCEMENT
	channelTypeStage    = 13 // GUILD_STAGE_VOICE
	channelTypeForum    = 15 // GUILD_FORUM
	channelTypeMedia    = 16 // GUILD_MEDIA
)

// permissionOverwrite is a channel permission overwrite (allow/deny are decimal
// string bitfields; type is 0 for a role, 1 for a member).
type permissionOverwrite struct {
	ID    string `json:"id"`
	Type  int    `json:"type"`
	Allow string `json:"allow"`
	Deny  string `json:"deny"`
}

// channelAttributes mirrors the common fields of a Discord channel object across
// the guild channel kinds. Kind-specific nested fields (forum tags, default
// reaction) live on the forum resource's own struct.
type channelAttributes struct {
	ID                            string                `json:"id"`
	Type                          int                   `json:"type"`
	GuildID                       *string               `json:"guild_id"`
	Name                          *string               `json:"name"`
	Topic                         *string               `json:"topic"`
	NSFW                          bool                  `json:"nsfw"`
	Position                      int64                 `json:"position"`
	ParentID                      *string               `json:"parent_id"`
	RateLimitPerUser              int64                 `json:"rate_limit_per_user"`
	Bitrate                       int64                 `json:"bitrate"`
	UserLimit                     int64                 `json:"user_limit"`
	RTCRegion                     *string               `json:"rtc_region"`
	VideoQualityMode              int64                 `json:"video_quality_mode"`
	DefaultAutoArchiveDuration    int64                 `json:"default_auto_archive_duration"`
	DefaultThreadRateLimitPerUser int64                 `json:"default_thread_rate_limit_per_user"`
	PermissionOverwrites          []permissionOverwrite `json:"permission_overwrites"`
}

// guildChannelsPath is the create/list endpoint for a guild's channels.
func guildChannelsPath(guildID string) string {
	return "/guilds/" + guildID + "/channels"
}

// channelPath is the read/update/delete endpoint for a single channel.
func channelPath(id string) string {
	return "/channels/" + id
}

// readChannel GETs a channel by id.
func readChannel(ctx context.Context, c *client.Client, id string) (*channelAttributes, error) {
	var a channelAttributes
	if err := c.Get(ctx, channelPath(id), &a); err != nil {
		return nil, err
	}
	return &a, nil
}

// syncPermsWithCategory emulates Discord's "sync permissions to category" action
// by copying the parent category's permission overwrites onto the channel. It is
// a no-op when the channel has no parent. Known gotcha: it conflicts with
// explicit discord_channel_permission overwrites on the same channel — own the
// overwrites either via the category (sync) or per-channel, not both.
func syncPermsWithCategory(ctx context.Context, c *client.Client, channelID, parentID string) error {
	if parentID == "" {
		return nil
	}
	parent, err := readChannel(ctx, c, parentID)
	if err != nil {
		return err
	}
	return c.Write(ctx, "PATCH", channelPath(channelID), map[string]any{"permission_overwrites": parent.PermissionOverwrites}, nil)
}
