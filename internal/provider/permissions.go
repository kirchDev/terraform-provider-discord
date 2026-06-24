package provider

import "math/big"

// permissionFlag pairs the provider's snake_case permission key with its Discord
// bit shift (the N in 1<<N). Discord serialises a permission set as a *decimal
// string* (a 64-bit field would lose precision as a JSON number), so the provider
// models every permission value — role permissions, @everyone permissions, and
// channel-overwrite allow/deny — as a decimal string too.
//
// Source: Discord API "Permissions" reference. Order mirrors the bit layout.
type permissionFlag struct {
	Key   string
	Shift uint
}

// permissionFlags is the full set the discord_permission helper understands.
// Covers general/server, text, voice and stage permissions.
var permissionFlags = []permissionFlag{
	// General / server
	{"create_instant_invite", 0},
	{"kick_members", 1},
	{"ban_members", 2},
	{"administrator", 3},
	{"manage_channels", 4},
	{"manage_guild", 5},
	{"add_reactions", 6},
	{"view_audit_log", 7},
	{"priority_speaker", 8},
	{"stream", 9},
	{"view_channel", 10},
	{"send_messages", 11},
	{"send_tts_messages", 12},
	{"manage_messages", 13},
	{"embed_links", 14},
	{"attach_files", 15},
	{"read_message_history", 16},
	{"mention_everyone", 17},
	{"use_external_emojis", 18},
	{"view_guild_insights", 19},
	{"connect", 20},
	{"speak", 21},
	{"mute_members", 22},
	{"deafen_members", 23},
	{"move_members", 24},
	{"use_vad", 25},
	{"change_nickname", 26},
	{"manage_nicknames", 27},
	{"manage_roles", 28},
	{"manage_webhooks", 29},
	{"manage_emojis", 30}, // MANAGE_GUILD_EXPRESSIONS
	{"use_application_commands", 31},
	{"request_to_speak", 32},
	{"manage_events", 33},
	{"manage_threads", 34},
	{"create_public_threads", 35},
	{"create_private_threads", 36},
	{"use_external_stickers", 37},
	{"send_thread_messages", 38},      // SEND_MESSAGES_IN_THREADS
	{"start_embedded_activities", 39}, // USE_EMBEDDED_ACTIVITIES
	{"moderate_members", 40},
	{"view_monetization_analytics", 41}, // VIEW_CREATOR_MONETIZATION_ANALYTICS
	{"use_soundboard", 42},
	{"create_expressions", 43}, // CREATE_GUILD_EXPRESSIONS
	{"create_events", 44},
	{"use_external_sounds", 45},
	{"send_voice_messages", 46},
	{"set_voice_channel_status", 48},
	{"send_polls", 49},
	{"use_external_apps", 50},
	{"pin_messages", 51},
	{"bypass_slowmode", 52},
}

// permissionKeys returns the permission keys in bit order.
func permissionKeys() []string {
	keys := make([]string, len(permissionFlags))
	for i, f := range permissionFlags {
		keys[i] = f.Key
	}
	return keys
}

// permissionBit returns 1<<Shift as a big.Int for the named permission key, and
// whether the key is known.
func permissionBit(key string) (*big.Int, bool) {
	for _, f := range permissionFlags {
		if f.Key == key {
			return new(big.Int).Lsh(big.NewInt(1), f.Shift), true
		}
	}
	return nil, false
}
