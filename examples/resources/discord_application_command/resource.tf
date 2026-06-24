# Guild-scoped slash command. options_json carries the option list as raw JSON.
resource "discord_application_command" "ping" {
  application_id = "567890123456789012"
  guild_id       = "123456789012345678"
  name           = "ping"
  type           = 1 # CHAT_INPUT
  description    = "Replies with pong."
  options_json   = jsonencode([])
  dm_permission  = false
}
