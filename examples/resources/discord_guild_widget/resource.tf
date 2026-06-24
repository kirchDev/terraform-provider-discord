resource "discord_guild_widget" "main" {
  server_id  = "123456789012345678"
  enabled    = true
  channel_id = "456789012345678901" # invite channel shown on the widget
}
