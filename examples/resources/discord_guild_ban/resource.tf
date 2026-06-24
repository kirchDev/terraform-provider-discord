resource "discord_guild_ban" "spammer" {
  server_id              = "123456789012345678"
  user_id                = "345678901234567890"
  reason                 = "Spamming invite links."
  delete_message_seconds = 86400 # purge last 24h of messages
}
