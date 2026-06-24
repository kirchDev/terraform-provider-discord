resource "discord_media_channel" "gallery" {
  server_id                          = "123456789012345678"
  name                               = "gallery"
  category                           = "234567890123456789"
  topic                              = "Share your media here."
  default_thread_rate_limit_per_user = 10
}
