data "discord_server" "main" {
  server_id = "123456789012345678"
}

output "server_name" {
  value = data.discord_server.main.name
}
