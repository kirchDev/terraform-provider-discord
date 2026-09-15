# A forum with two tags.
#
# Every tag carries a `key` you choose. Discord never sees it: it is what lets
# the provider keep the id Discord assigned when you rename a tag, change its
# emoji, toggle `moderated`, reorder the list, or insert another tag above it —
# so posts keep their tags. Pick keys you will not want to change — changing one
# retires that tag and creates a new one with a fresh id, which strips it from
# every post that carried it.
resource "discord_forum_channel" "help" {
  server_id            = "123456789012345678"
  name                 = "help"
  category             = "234567890123456789"
  topic                = "Ask questions and get help."
  default_sort_order   = 0
  default_forum_layout = 1

  # Every post must carry at least one tag, so a discord_forum_post into this
  # forum must set `tags`.
  require_tag = true

  available_tags = [
    {
      key       = "unresolved"
      name      = "Unresolved"
      moderated = false
    },
    {
      key       = "resolved"
      name      = "Resolved"
      moderated = true
    },
  ]
}
