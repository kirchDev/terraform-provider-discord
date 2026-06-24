resource "discord_thread" "release_notes" {
  channel_id            = "456789012345678901"
  name                  = "Release Notes"
  auto_archive_duration = 1440 # minutes
  rate_limit_per_user   = 10
  invitable             = true
}
