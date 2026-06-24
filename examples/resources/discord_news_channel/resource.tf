resource "discord_news_channel" "announcements" {
  server_id = "123456789012345678"
  name      = "announcements"
  category  = "234567890123456789"
  topic     = "Official updates only."
}
