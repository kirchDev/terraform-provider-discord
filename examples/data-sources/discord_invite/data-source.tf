data "discord_invite" "main" {
  code = "abcdef"
}

output "invite_uses" {
  value = data.discord_invite.main.uses
}
