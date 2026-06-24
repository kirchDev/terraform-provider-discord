data "discord_emoji" "party" {
  server_id = "123456789012345678"
  emoji_id  = "234567890123456789"
}

output "party_emoji_name" {
  value = data.discord_emoji.party.name
}
