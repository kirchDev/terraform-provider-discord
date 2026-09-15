resource "discord_invite" "welcome" {
  channel_id = "456789012345678901"
  max_age    = 86400 # 1 day, 0 = never expires
  max_uses   = 25    # 0 = unlimited
  temporary  = false
  unique     = true
}

# A permanent invite that grants an opt-in role to whoever joins through it.
# The bot needs Manage Roles, and the role must sit below the bot's own role.
resource "discord_invite" "minecraft" {
  channel_id = "456789012345678901"
  max_age    = 0
  role_ids   = [discord_role.minecraft.id]
}
