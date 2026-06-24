resource "discord_webhook" "deploys" {
  channel_id = "456789012345678901"
  name       = "Deploy Bot"
}

output "deploy_webhook_url" {
  value     = discord_webhook.deploys.url
  sensitive = true
}
