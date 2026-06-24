# Order the top level (categories and uncategorised channels). Leave the
# per-channel `position` unset and let this resource own the ordering.
resource "discord_channel_order" "top_level" {
  server_id = "123456789012345678"
  channel_ids = [
    "111111111111111111", # info category
    "222222222222222222", # general category
    "333333333333333333", # voice category
  ]
}

# Order the channels inside one category.
resource "discord_channel_order" "general" {
  server_id = "123456789012345678"
  parent_id = "222222222222222222"
  channel_ids = [
    "444444444444444444", # rules
    "555555555555555555", # announcements
    "666666666666666666", # chat
  ]
}
