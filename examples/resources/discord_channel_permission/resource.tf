# Set a permission overwrite on a channel using named permission keys.
data "discord_permission" "muted" {
  deny = ["send_messages", "add_reactions", "speak"]
}

resource "discord_channel_permission" "muted_role" {
  channel_id   = "456789012345678901"
  overwrite_id = "234567890123456789" # role or member snowflake
  type         = "role"
  allow        = data.discord_permission.muted.allow_bits
  deny         = data.discord_permission.muted.deny_bits
}
