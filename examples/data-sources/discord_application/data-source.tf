# The bot's own application — use its id for discord_application_command
# instead of hardcoding the application id.
data "discord_application" "this" {}

resource "discord_application_command" "ping" {
  application_id = data.discord_application.this.id
  name           = "ping"
  description     = "Replies with pong"
}
