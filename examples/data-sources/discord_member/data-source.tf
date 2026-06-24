data "discord_member" "alice" {
  server_id = "123456789012345678"
  user_id   = "345678901234567890"
}

output "alice_roles" {
  value = data.discord_member.alice.roles
}
