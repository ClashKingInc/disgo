package discord

import (
	"fmt"
	"time"

	"github.com/disgoorg/json/v2"
	"github.com/disgoorg/snowflake/v2"
)

// UnknownChannel retains an unrecognized channel without inventing messaging
// capabilities. Discord can add channel types before a client release; one
// such channel must not invalidate the enclosing guild and reconnect its shard.
type UnknownChannel struct {
	raw        json.RawMessage
	id         snowflake.ID
	kind       ChannelType
	name       string
	guildID    snowflake.ID
	parentID   *snowflake.ID
	position   int
	overwrites PermissionOverwrites
}

func (c *UnknownChannel) UnmarshalJSON(data []byte) error {
	var v struct {
		ID         snowflake.ID                   `json:"id"`
		Type       ChannelType                    `json:"type"`
		Name       string                         `json:"name"`
		GuildID    snowflake.ID                   `json:"guild_id"`
		ParentID   *snowflake.ID                  `json:"parent_id"`
		Position   int                            `json:"position"`
		Overwrites []UnmarshalPermissionOverwrite `json:"permission_overwrites"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	c.raw = append(c.raw[:0], data...)
	c.id, c.kind, c.name, c.guildID, c.parentID, c.position = v.ID, v.Type, v.Name, v.GuildID, v.ParentID, v.Position
	c.overwrites = parsePermissionOverwrites(v.Overwrites)
	return nil
}
func (c UnknownChannel) MarshalJSON() ([]byte, error) { return append([]byte(nil), c.raw...), nil }
func (c UnknownChannel) ID() snowflake.ID             { return c.id }
func (c UnknownChannel) Type() ChannelType            { return c.kind }
func (c UnknownChannel) Name() string                 { return c.name }
func (c UnknownChannel) CreatedAt() time.Time         { return c.id.Time() }
func (c UnknownChannel) String() string               { return channelString(c) }
func (UnknownChannel) channel()                       {}

// UnknownGuildChannel is an opaque channel whose guild membership is known
// from guild_id or its enclosing guild payload. It intentionally implements
// neither MessageChannel nor GuildMessageChannel.
type UnknownGuildChannel struct{ UnknownChannel }

func (c UnknownGuildChannel) GuildID() snowflake.ID                      { return c.guildID }
func (c UnknownGuildChannel) ParentID() *snowflake.ID                    { return c.parentID }
func (c UnknownGuildChannel) Position() int                              { return c.position }
func (c UnknownGuildChannel) PermissionOverwrites() PermissionOverwrites { return c.overwrites }
func (c UnknownGuildChannel) Mention() string                            { return ChannelMention(c.id) }
func (UnknownGuildChannel) guildChannel()                                {}

// ResolveGuildChannel uses explicit enclosing guild context for unknown types.
// Known direct-message channels remain invalid in a guild response.
func ResolveGuildChannel(channel Channel, guildID snowflake.ID) (GuildChannel, error) {
	if channel == nil {
		return nil, fmt.Errorf("missing channel")
	}
	if c, ok := channel.(GuildChannel); ok {
		return c, nil
	}
	if c, ok := channel.(UnknownChannel); ok && guildID != 0 {
		if c.guildID != 0 && c.guildID != guildID {
			return nil, fmt.Errorf("channel guild does not match enclosing guild")
		}
		c.guildID = guildID
		return UnknownGuildChannel{c}, nil
	}
	return nil, fmt.Errorf("channel type %d has no guild context", channel.Type())
}

var _ Channel = UnknownChannel{}
var _ GuildChannel = UnknownGuildChannel{}
