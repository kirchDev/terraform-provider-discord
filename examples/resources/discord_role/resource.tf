data "discord_permission" "mod" {
  allow = ["view_channel", "send_messages", "manage_messages", "kick_members"]
}

data "discord_color" "blurple" {
  hex = "#5865F2"
}

resource "discord_role" "moderator" {
  server_id   = "123456789012345678"
  name        = "Moderator"
  permissions = data.discord_permission.mod.allow_bits
  color       = data.discord_color.blurple.dec
  hoist       = true
  mentionable = true
}
