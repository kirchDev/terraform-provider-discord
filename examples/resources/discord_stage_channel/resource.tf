resource "discord_stage_channel" "town_hall" {
  server_id  = "123456789012345678"
  name       = "Town Hall"
  category   = "234567890123456789"
  bitrate    = 64000
  user_limit = 1000
}
