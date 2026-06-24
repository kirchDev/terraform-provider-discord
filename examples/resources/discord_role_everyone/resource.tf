# Manages the base @everyone role permissions for a guild.
data "discord_permission" "everyone" {
  allow = ["view_channel", "send_messages", "read_message_history"]
}

resource "discord_role_everyone" "main" {
  server_id   = "123456789012345678"
  permissions = data.discord_permission.everyone.allow_bits
}
