resource "discord_category_channel" "general" {
  server_id = "123456789012345678"
  name      = "General"
}

resource "discord_text_channel" "chat" {
  server_id           = "123456789012345678"
  name                = "chat"
  category            = discord_category_channel.general.id
  topic               = "Talk about anything here."
  rate_limit_per_user = 5
  nsfw                = false
}
