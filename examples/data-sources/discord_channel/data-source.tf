data "discord_channel" "general" {
  channel_id = "456789012345678901"
}

output "general_name" {
  value = data.discord_channel.general.name
}
