# discord_managed_server adopts an existing guild — you cannot create a guild
# via the API, so import the server you already own and manage it from there.
# import first: tofu import discord_managed_server.main 123456789012345678
resource "discord_managed_server" "main" {
  server_id                    = "123456789012345678"
  name                         = "My Community"
  description                  = "A friendly place to hang out."
  verification_level           = 1
  premium_progress_bar_enabled = true

  lifecycle {
    prevent_destroy = true
  }
}
