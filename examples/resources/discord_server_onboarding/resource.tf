# prompts_json carries the onboarding prompts as a raw JSON string.
resource "discord_server_onboarding" "main" {
  server_id           = "123456789012345678"
  enabled             = true
  mode                = 0
  default_channel_ids = ["456789012345678901"]
  prompts_json        = jsonencode([])
}
