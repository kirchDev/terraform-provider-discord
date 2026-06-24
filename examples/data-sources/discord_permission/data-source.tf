# Compute permission bitfields from named keys instead of magic decimal masks.
data "discord_permission" "mod" {
  allow = ["view_channel", "send_messages"]
  deny  = ["mention_everyone"]
}

# Feed the computed bits into a role or channel overwrite, e.g.
# permissions = data.discord_permission.mod.allow_bits
output "mod_allow_bits" {
  value = data.discord_permission.mod.allow_bits
}
