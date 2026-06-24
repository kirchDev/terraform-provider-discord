data "discord_user" "alice" {
  user_id = "345678901234567890"
}

output "alice_username" {
  value = data.discord_user.alice.username
}
