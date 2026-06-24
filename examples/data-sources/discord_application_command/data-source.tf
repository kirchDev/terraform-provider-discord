data "discord_application_command" "ping" {
  application_id = "123456789012345678"
  guild_id       = "234567890123456789"
  command_id     = "345678901234567890"
}

output "ping_command_name" {
  value = data.discord_application_command.ping.name
}
