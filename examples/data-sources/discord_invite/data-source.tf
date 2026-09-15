data "discord_invite" "main" {
  code = "abcdef"
}

output "invite_uses" {
  value = data.discord_invite.main.uses
}

output "invite_role_ids" {
  value = data.discord_invite.main.role_ids
}
