# A fixed, declarative post in a forum (or media) channel — e.g. a pinned
# guidelines / info post that should always be at the top of the forum.
resource "discord_forum_post" "guidelines" {
  channel_id = "123456789012345678" # a discord_forum_channel id
  name       = "📌 Read me first"
  content    = "Welcome! Please read the guidelines before opening a post."
  pinned     = true
}
