resource "discord_welcome_screen" "main" {
  server_id   = "123456789012345678"
  enabled     = true
  description = "Welcome! Check out these channels to get started."

  welcome_channels = [
    {
      channel_id  = "456789012345678901"
      description = "Read the rules"
      emoji_name  = "📜"
    },
    {
      channel_id  = "456789012345678902"
      description = "Introduce yourself"
      emoji_name  = "👋"
    },
  ]
}
