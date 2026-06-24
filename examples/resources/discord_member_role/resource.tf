# Assign a single role to a member without owning their whole role set — this
# coexists with bots that also grant roles (level/reaction-role bots). For
# exclusive ownership of a member's entire role set, use discord_member_roles.
resource "discord_member_role" "vip" {
  server_id = "123456789012345678"
  user_id   = "234567890123456789"
  role_id   = "345678901234567890"
}
