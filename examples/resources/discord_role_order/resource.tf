# Order the role hierarchy from highest to lowest. Leave the per-role `position`
# unset and let this resource own the ordering. List only roles below the bot's
# own highest role, and never @everyone.
resource "discord_role_order" "hierarchy" {
  server_id = "123456789012345678"
  role_ids = [
    "111111111111111111", # admin (highest)
    "222222222222222222", # moderator
    "333333333333333333", # member (just above @everyone)
  ]
}
