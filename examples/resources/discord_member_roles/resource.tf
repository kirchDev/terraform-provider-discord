# Manages the full set of roles assigned to a guild member.
resource "discord_member_roles" "alice" {
  server_id = "123456789012345678"
  user_id   = "345678901234567890"
  roles = [
    "234567890123456789",
    "234567890123456790",
  ]
}
