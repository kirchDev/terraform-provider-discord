resource "discord_voice_channel" "lounge" {
  server_id  = "123456789012345678"
  name       = "Lounge"
  category   = "234567890123456789"
  bitrate    = 64000
  user_limit = 10
}
