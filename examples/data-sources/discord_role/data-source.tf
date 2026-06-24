data "discord_role" "moderator" {
  server_id = "123456789012345678"
  role_id   = "234567890123456789"
}

output "moderator_permissions" {
  value = data.discord_role.moderator.permissions
}
