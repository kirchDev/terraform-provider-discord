# External event (entity_type 3) hosted at a location.
resource "discord_scheduled_event" "meetup" {
  server_id            = "123456789012345678"
  name                 = "Community Meetup"
  description          = "Monthly hangout for members."
  entity_type          = 3
  scheduled_start_time = "2026-07-01T18:00:00Z"
  scheduled_end_time   = "2026-07-01T20:00:00Z"
  location             = "https://meet.example.com/community"
}
