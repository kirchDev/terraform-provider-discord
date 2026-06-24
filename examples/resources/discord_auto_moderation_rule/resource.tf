# Keyword filter rule (trigger_type 1) that blocks matching messages.
resource "discord_auto_moderation_rule" "no_spam" {
  server_id      = "123456789012345678"
  name           = "Block banned words"
  event_type     = 1 # message send
  trigger_type   = 1 # keyword
  enabled        = true
  keyword_filter = ["badword", "anotherword"]

  actions = [
    {
      type = 1 # block message
    },
    {
      type       = 2 # send alert
      channel_id = "456789012345678901"
    },
  ]
}
