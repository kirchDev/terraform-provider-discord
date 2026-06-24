resource "discord_invite" "welcome" {
  channel_id = "456789012345678901"
  max_age    = 86400 # 1 day, 0 = never expires
  max_uses   = 25    # 0 = unlimited
  temporary  = false
  unique     = true
}
