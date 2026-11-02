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

# Declare every permission overwrite on a channel; any others are removed.
resource "discord_text_channel" "private" {
  server_id = "123456789012345678"
  name      = "private"

  permissions = {
    role_overwrites = {
      "234567890123456789" = {
        allow = ["view_channel", "send_messages"]
        deny  = ["mention_everyone"]
      }
    }
    user_overwrites = {
      "345678901234567890" = {
        allow = ["view_channel"]
      }
    }
  }
}
