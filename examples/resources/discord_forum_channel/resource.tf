resource "discord_forum_channel" "help" {
  server_id            = "123456789012345678"
  name                 = "help"
  category             = "234567890123456789"
  topic                = "Ask questions and get help."
  default_sort_order   = 0
  default_forum_layout = 1

  available_tags = [
    {
      name      = "Unresolved"
      moderated = false
    },
    {
      name      = "Resolved"
      moderated = true
    },
  ]
}
